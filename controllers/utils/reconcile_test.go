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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/backube/volsync/controllers/utils"
)

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

			op, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, job, logger, func() error {
				job.Labels = map[string]string{
					"a": "b",
					"c": "d",
				}

				job.Spec.BackoffLimit = &backoffLimit
				job.Spec.Parallelism = &parallellism

				job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever

				job.Spec.Template.ObjectMeta.Name = "jobpod1"
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
				op, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, job, logger, func() error {
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
				op, err := utils.CreateOrUpdateDeleteOnImmutableErr(ctx, k8sClient, job, logger, func() error {
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

				Expect(op).To(Equal(ctrlutil.OperationResultNone))

				// job should no longer exist
				reloadErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), testJob)
				Expect(kerrors.IsNotFound(reloadErr)).To(BeTrue())
			})
		})
	})
})
