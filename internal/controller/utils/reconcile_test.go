/*
Copyright 2022 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package utils_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/backube/volsync/internal/controller/utils"
)

const (
	testJobPodLabelKey   = "job-test"
	testJobPodLabelValue = "owned"
)

type listErrorClient struct {
	client.Client
}

func (listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("injected Pod list failure")
}

var _ = Describe("JobHasActiveOrTerminatingPods", func() {
	var testNamespace *corev1.Namespace
	var job *batchv1.Job
	var createPod func(string, corev1.PodPhase, bool) *corev1.Pod

	BeforeEach(func() {
		testNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "job-pods-"}}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "job-", Namespace: testNamespace.Name},
			Spec: batchv1.JobSpec{
				ManualSelector: ptr.To(true),
				BackoffLimit:   ptr.To[int32](2),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					testJobPodLabelKey: testJobPodLabelValue,
				}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						testJobPodLabelKey: testJobPodLabelValue,
					}},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers:    []corev1.Container{{Name: "job-container", Image: "test-image"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())

		createPod = func(name string, phase corev1.PodPhase, owned bool) *corev1.Pod {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: testNamespace.Name,
					Labels: map[string]string{
						testJobPodLabelKey: testJobPodLabelValue,
					},
					Finalizers: []string{"test.volsync.io/keep"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "job-container", Image: "test-image"}},
				},
			}
			if owned {
				pod.OwnerReferences = []metav1.OwnerReference{
					*metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job")),
				}
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			pod.Status.Phase = phase
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			Expect(pod.DeletionTimestamp).NotTo(BeNil())
			return pod
		}
	})

	AfterEach(func() {
		pods := &corev1.PodList{}
		Expect(k8sClient.List(ctx, pods, client.InNamespace(testNamespace.Name))).To(Succeed())
		for i := range pods.Items {
			pod := &pods.Items[i]
			pod.Finalizers = nil
			Expect(k8sClient.Update(ctx, pod)).To(Succeed())
			if pod.DeletionTimestamp == nil {
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}
		}
		Expect(k8sClient.Delete(ctx, job)).To(Succeed())
		Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
	})

	It("detects active and terminating status without listing Pods", func() {
		active, err := utils.JobHasActiveOrTerminatingPods(ctx, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeFalse())

		job.Status.Active = 1
		active, err = utils.JobHasActiveOrTerminatingPods(ctx, listErrorClient{}, job)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeTrue())

		job.Status.Active = 0
		terminating := int32(1)
		job.Status.Terminating = &terminating
		active, err = utils.JobHasActiveOrTerminatingPods(ctx, listErrorClient{}, job)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeTrue())
	})

	It("waits without listing for deferred, active, terminating, and unfinished Jobs", func() {
		job.Status.Succeeded = 1
		inProgress, err := utils.JobShouldWaitForCompletion(ctx, listErrorClient{}, job, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeTrue())

		job.Status.Active = 1
		inProgress, err = utils.JobShouldWaitForCompletion(ctx, listErrorClient{}, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeTrue())

		job.Status.Active = 0
		terminating := int32(1)
		job.Status.Terminating = &terminating
		inProgress, err = utils.JobShouldWaitForCompletion(ctx, listErrorClient{}, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeTrue())

		job.Status.Terminating = nil
		job.Status.Succeeded = 0
		job.Status.Failed = 1
		inProgress, err = utils.JobShouldWaitForCompletion(ctx, listErrorClient{}, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeTrue())
	})

	It("detects deleting Pending and Running Pods when Terminating is nil", func() {
		job.Status.Succeeded = 1
		for i, phase := range []corev1.PodPhase{corev1.PodPending, corev1.PodRunning} {
			createPod(fmt.Sprintf("pod-%d", i), phase, true)
			Expect(job.Status.Active).To(Equal(int32(0)))
			Expect(job.Status.Terminating).To(BeNil())

			active, err := utils.JobHasActiveOrTerminatingPods(ctx, k8sClient, job)
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeTrue())
			inProgress, err := utils.JobShouldWaitForCompletion(ctx, k8sClient, job, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(inProgress).To(BeTrue())

			pod := &corev1.Pod{}
			key := types.NamespacedName{
				Namespace: testNamespace.Name,
				Name:      fmt.Sprintf("pod-%d", i),
			}
			Expect(k8sClient.Get(ctx, key, pod)).To(Succeed())
			pod.Finalizers = nil
			Expect(k8sClient.Update(ctx, pod)).To(Succeed())
		}
	})

	It("detects deleting Pods when Terminating is explicitly zero", func() {
		job.Status.Succeeded = 1
		pod := createPod("pod-terminating-zero", corev1.PodRunning, true)
		Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
		Expect(pod.DeletionTimestamp).NotTo(BeNil())
		Expect(metav1.IsControlledBy(pod, job)).To(BeTrue())

		job.Status.Active = 0
		terminating := int32(0)
		job.Status.Terminating = &terminating

		active, err := utils.JobHasActiveOrTerminatingPods(ctx, k8sClient, job)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeTrue())
		inProgress, err := utils.JobShouldWaitForCompletion(ctx, k8sClient, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeTrue())
	})

	It("ignores terminal Pods and Pods not controlled by the Job", func() {
		job.Status.Succeeded = 1
		unowned := createPod("unowned-pending", corev1.PodPending, false)
		inProgress, err := utils.JobShouldWaitForCompletion(ctx, k8sClient, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeFalse())
		unowned.Finalizers = nil
		Expect(k8sClient.Update(ctx, unowned)).To(Succeed())

		createPod("succeeded", corev1.PodSucceeded, true)
		createPod("failed", corev1.PodFailed, true)
		inProgress, err = utils.JobShouldWaitForCompletion(ctx, k8sClient, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeFalse())
	})

	It("does not wait when an exhausted Job has no active Pods", func() {
		job.Status.Failed = *job.Spec.BackoffLimit
		inProgress, err := utils.JobShouldWaitForCompletion(ctx, k8sClient, job, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(inProgress).To(BeFalse())
	})

	It("fails closed when Pod listing fails", func() {
		active, err := utils.JobHasActiveOrTerminatingPods(ctx, nil, job)
		Expect(err).To(MatchError(ContainSubstring("API reader is nil")))
		Expect(active).To(BeTrue())

		active, err = utils.JobHasActiveOrTerminatingPods(ctx, listErrorClient{}, job)
		Expect(err).To(MatchError(ContainSubstring("injected Pod list failure")))
		Expect(active).To(BeTrue())

		job.Status.Succeeded = 1
		inProgress, err := utils.JobShouldWaitForCompletion(ctx, nil, job, false)
		Expect(err).To(MatchError(ContainSubstring("API reader is nil")))
		Expect(inProgress).To(BeTrue())

		inProgress, err = utils.JobShouldWaitForCompletion(ctx, listErrorClient{}, job, false)
		Expect(err).To(MatchError(ContainSubstring("injected Pod list failure")))
		Expect(inProgress).To(BeTrue())
	})
})

var _ = Describe("CreateOrUpdateDeleteOnImmutableErr", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	const testJobName = "job-for-cr-update-imm-test"
	var testJob *batchv1.Job
	var testNamespace *corev1.Namespace

	BeforeEach(func() {
		// Create namespace for test
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "myns-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, testNamespace)).To(Succeed())
	})

	Context("When the resource does not already exist", func() {
		BeforeEach(func() {
			backoffLimit := int32(2)
			parallellism := int32(1)

			// job to be used for these CreateOrUpdateDeleteOnImmutableErr tests
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testJobName,
					Namespace: testNamespace.GetName(),
				},
			}

			logger := logger.WithValues("job", client.ObjectKeyFromObject(job))

			op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, k8sClient, job, logger, func() error {
				job.Labels = map[string]string{
					"a": "b",
					"c": "d",
				}

				job.Spec.BackoffLimit = &backoffLimit
				job.Spec.Parallelism = &parallellism

				job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever

				job.Spec.Template.Name = "jobpod1"
				job.Spec.Template.Labels = map[string]string{
					"podlabela": "aa",
					"podlabelb": "bb",
				}

				job.Spec.Template.Spec.Containers = []corev1.Container{{
					Name:    "testcontainer1",
					Command: []string{"/bin/dosomething"},
					Image:   "fakeimagerepo/testing/tester:latest",
				}}

				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(deferred).To(BeFalse())
			Expect(op).To(Equal(ctrlutil.OperationResultCreated))

			testJob = &batchv1.Job{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)).To(Succeed())
		})

		It("Should create the job", func() {
			Expect(testJob.Generation).To(Equal(int64(1)))
		})

		Context("When updating a resource that already exists", func() {
			It("Should update the resource when no immutable fields are modified", func() {
				backoffLimitUpdated := int32(6)
				parallelismUpdated := int32(0)

				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testJobName,
						Namespace: testNamespace.GetName(),
					},
				}
				op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, k8sClient, job, logger, func() error {
					// Modifying labels should be no problem
					job.Labels = map[string]string{
						"a": "b",
						"c": "d",
						"e": "f",
					}

					// Backofflimit and Parallelism should be modifiable as well
					job.Spec.BackoffLimit = &backoffLimitUpdated
					job.Spec.Parallelism = &parallelismUpdated

					return nil
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(deferred).To(BeFalse())
				Expect(op).To(Equal(ctrlutil.OperationResultUpdated))

				// reload the job
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)).To(Succeed())

				Expect(testJob.Labels["c"]).To(Equal("d"))
				Expect(testJob.Labels["e"]).To(Equal("f"))

				Expect(*testJob.Spec.BackoffLimit).To(Equal(backoffLimitUpdated))
				Expect(*testJob.Spec.Parallelism).To(Equal(parallelismUpdated))
			})

			It("Should delete the resource if the update tries to update an immutable field", func() {
				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testJobName,
						Namespace: testNamespace.GetName(),
					},
				}
				op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, k8sClient, job, logger, func() error {
					// Job.Spec.Template is immutable
					job.Spec.Template.Spec.NodeName = "node-a"
					job.Spec.Template.Spec.Tolerations = []corev1.Toleration{
						{
							Key:      "example-key",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					}

					return nil
				})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Deleting object so it can be recreated"))
				Expect(deferred).To(BeFalse())

				Expect(op).To(Equal(ctrlutil.OperationResultNone))

				// job should no longer exist
				reloadErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)
				Expect(kerrors.IsNotFound(reloadErr)).To(BeTrue())
			})

			It("Should preserve an active job when an immutable field is changed", func() {
				testJob.Status.Active = 1
				Expect(k8sClient.Status().Update(ctx, testJob)).To(Succeed())
				originalTemplate := testJob.Spec.Template.DeepCopy()
				originalUID := testJob.UID

				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testJobName,
						Namespace: testNamespace.GetName(),
					},
				}
				op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, k8sClient, job, logger, func() error {
					// Job.Spec.Template is immutable.
					job.Spec.Template.Spec.NodeName = "node-b"
					return nil
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(deferred).To(BeTrue())
				Expect(op).To(Equal(ctrlutil.OperationResultNone))

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)).To(Succeed())
				Expect(testJob.UID).To(Equal(originalUID))
				Expect(testJob.DeletionTimestamp).To(BeNil())
				Expect(testJob.Status.Active).To(Equal(int32(1)))
				Expect(testJob.Spec.Template).To(Equal(*originalTemplate))
			})

			It("reports when an immutable update is deferred by the legacy Pod-list fallback", func() {
				Expect(testJob.Status.Active).To(Equal(int32(0)))
				Expect(testJob.Status.Terminating).To(BeNil())

				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "running-pod",
						Namespace: testNamespace.Name,
						Labels:    testJob.Spec.Template.Labels,
						OwnerReferences: []metav1.OwnerReference{
							*metav1.NewControllerRef(testJob, batchv1.SchemeGroupVersion.WithKind("Job")),
						},
					},
					Spec:   testJob.Spec.Template.Spec,
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				}
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testJobName,
						Namespace: testNamespace.GetName(),
					},
				}
				op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx,
					listErrorClient{k8sClient}, k8sClient, job, logger, func() error {
						job.Spec.Template.Spec.NodeName = "node-d"
						return nil
					})

				Expect(err).NotTo(HaveOccurred())
				Expect(deferred).To(BeTrue())
				Expect(op).To(Equal(ctrlutil.OperationResultNone))
			})

			It("Should preserve a terminating job when an immutable field is changed", func() {
				terminating := int32(1)
				testJob.Status.Terminating = &terminating
				Expect(k8sClient.Status().Update(ctx, testJob)).To(Succeed())
				originalTemplate := testJob.Spec.Template.DeepCopy()
				originalUID := testJob.UID

				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testJobName,
						Namespace: testNamespace.GetName(),
					},
				}
				op, deferred, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, k8sClient, job, logger, func() error {
					// Job.Spec.Template is immutable.
					job.Spec.Template.Spec.NodeName = "node-c"
					return nil
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(deferred).To(BeTrue())
				Expect(op).To(Equal(ctrlutil.OperationResultNone))

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)).To(Succeed())
				Expect(testJob.UID).To(Equal(originalUID))
				Expect(testJob.DeletionTimestamp).To(BeNil())
				Expect(testJob.Status.Terminating).NotTo(BeNil())
				Expect(*testJob.Status.Terminating).To(Equal(terminating))
				Expect(testJob.Spec.Template).To(Equal(*originalTemplate))
			})
		})
	})
})
