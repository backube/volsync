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
	"os"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/backube/volsync/internal/controller/utils"
)

const (
	timeout  = "10s"
	interval = "1s"
)

var _ = Describe("Pod Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	var ns *corev1.Namespace
	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "namespc-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())
	})
	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	When("A job exists with pods", func() {
		var job *batchv1.Job
		BeforeEach(func() {
			jobName := "job-logs-testjob"
			// job to be used for these pod logs tests
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: ns.GetName(),
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name: jobName,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "test",
									Command: []string{"/bin/fakemovercmd.sh"},
									Image:   "someimagepath:here",
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, job)).To(Succeed())
		})

		When("Job has pods", func() {
			var pod1Running, pod2Running, pod3Running *corev1.Pod
			var pod4Succeeded, pod5Succeeded *corev1.Pod
			var pod6Failed, pod7Failed *corev1.Pod

			BeforeEach(func() {
				pod1Running = createTestPodForJob("test1", ns.GetName(), job.GetName(), corev1.PodRunning)
				time.Sleep(2 * time.Second) // Make sure the pods will have different creation timestamps
				pod2Running = createTestPodForJob("test2", ns.GetName(), job.GetName(), corev1.PodRunning)
				time.Sleep(2 * time.Second)
				pod3Running = createTestPodForJob("test3", ns.GetName(), job.GetName(), corev1.PodRunning)

				// Make pod for different job - should not be picked up by GetPodsForJob()
				createTestPodForJob("testpod-diff-job", ns.GetName(), "some-other-job", corev1.PodRunning)

				// Make pod in pending phse - should not be picked up by GetPodsForJob()
				createTestPodForJob("testpod-pending", ns.GetName(), job.GetName(), corev1.PodPending)

				Expect(pod1Running.CreationTimestamp.Before(&pod2Running.CreationTimestamp)).To(BeTrue())
				Expect(pod1Running.CreationTimestamp.Before(&pod3Running.CreationTimestamp)).To(BeTrue())
				Expect(pod2Running.CreationTimestamp.Before(&pod3Running.CreationTimestamp)).To(BeTrue())

				pod4Succeeded = createTestPodForJob("test4", ns.GetName(), job.GetName(), corev1.PodSucceeded)
				time.Sleep(2 * time.Second)
				pod5Succeeded = createTestPodForJob("test5", ns.GetName(), job.GetName(), corev1.PodSucceeded)
				time.Sleep(2 * time.Second) // Add sleep after pod5 to ensure timestamp difference

				Expect(pod4Succeeded.CreationTimestamp.Before(&pod5Succeeded.CreationTimestamp)).To(BeTrue())

				pod6Failed = createTestPodForJob("test6", ns.GetName(), job.GetName(), corev1.PodFailed)
				time.Sleep(2 * time.Second)
				pod7Failed = createTestPodForJob("test7", ns.GetName(), job.GetName(), corev1.PodFailed)
				time.Sleep(2 * time.Second) // Add sleep after pod7 to ensure timestamp difference

				Expect(pod6Failed.CreationTimestamp.Before(&pod7Failed.CreationTimestamp)).To(BeTrue())
			})

			It("Should get running/successful/failed pods for a specific job", func() {
				runningPods, successfulPods, failedPods, err := utils.GetPodsForJob(ctx, logger,
					job.GetName(), job.GetNamespace())
				Expect(err).NotTo(HaveOccurred())

				Expect(len(runningPods)).To(Equal(3))
				Expect(len(successfulPods)).To(Equal(2))
				Expect(len(failedPods)).To(Equal(2))
			})

			It("Should find the latest successful pod for a successful job", func() {
				pod, err := utils.GetNewestPodForJob(ctx, logger, job.GetName(), job.GetNamespace(), false)
				Expect(err).NotTo(HaveOccurred())

				Expect(pod).NotTo(BeNil())
				Expect(pod.GetName()).To(Equal(pod5Succeeded.GetName()))
			})

			It("Should find the latest failed pod for a failed job", func() {
				pod, err := utils.GetNewestPodForJob(ctx, logger, job.GetName(), job.GetNamespace(), true)
				Expect(err).NotTo(HaveOccurred())

				Expect(pod).NotTo(BeNil())
				Expect(pod.GetName()).To(Equal(pod7Failed.GetName()))
			})

			When("No failed pods exist for a job", func() {
				BeforeEach(func() {
					// Update the failed pods - set them to phase unknown
					pod6Failed.Status.Phase = corev1.PodUnknown
					Expect(k8sClient.Status().Update(ctx, pod6Failed)).To(Succeed())
					pod7Failed.Status.Phase = corev1.PodUnknown
					Expect(k8sClient.Status().Update(ctx, pod7Failed)).To(Succeed())

					// Make sure the updates are picked up by the clientset cache
					Eventually(func() bool {
						p6, err := k8sClientSet.CoreV1().Pods(ns.GetName()).Get(ctx, pod6Failed.GetName(), metav1.GetOptions{})
						if err != nil {
							return false
						}
						p7, err := k8sClientSet.CoreV1().Pods(ns.GetName()).Get(ctx, pod7Failed.GetName(), metav1.GetOptions{})
						if err != nil {
							return false
						}
						return p6.Status.Phase == corev1.PodUnknown && p7.Status.Phase == corev1.PodUnknown
					}, timeout, interval).Should(BeTrue())
				})

				It("Should find the latest running pod for a failed job", func() {
					pod, err := utils.GetNewestPodForJob(ctx, logger, job.GetName(), job.GetNamespace(), true)
					Expect(err).NotTo(HaveOccurred())

					Expect(pod).NotTo(BeNil())
					Expect(pod.GetName()).To(Equal(pod3Running.GetName()))
				})
			})
		})
	})
})

var _ = Describe("Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("Test log filtering", func() {
		// nolint:lll
		testLog := `Starting container
Testing mandatory env variables
== Checking directory for content ===
== Initialize Dir =======
created restic repository f5bccd54c8 at s3:http://minio-api-minio.apps.app-aws-411ga-sno-net2-zp5jq.dev06.red-chesterfield.com/ttest-restic-new

=== Starting backup ===
~/DEVFEDORA/volsync/TESTDATA ~/DEVFEDORA/volsync/RESTICTESTS
repository f5bccd54 opened (repository version 2) successfully, password is correct
created new cache in /home/testuser/DEVFEDORA/volsync/RESTICTESTS/CACHE
no parent snapshot found, will read all files

Files:          25 new,     0 changed,     0 unmodified
Dirs:            3 new,     0 changed,     0 unmodified
Added to the repository: 12.941 MiB (12.529 MiB stored)

processed 25 files, 36.658 MiB in 0:12
snapshot 0ff74383 saved
~/DEVFEDORA/volsync/RESTICTESTS
=== Starting forget ===
Restic completed in 18s
=== Done ===`

		It("Should retain all lines from the log when AllLines filter is used", func() {
			reader := strings.NewReader(testLog)
			filteredLines, err := utils.FilterLogs(reader, utils.AllLines)
			Expect(err).NotTo(HaveOccurred())

			// All lines should be returned
			Expect(filteredLines).To(Equal(testLog))
		})

		It("Should filter the logs when a log filter is used", func() {
			// nolint:lll
			expectedFilteredLog := `created restic repository f5bccd54c8 at s3:http://minio-api-minio.apps.app-aws-411ga-sno-net2-zp5jq.dev06.red-chesterfield.com/ttest-restic-new
=== Starting backup ===
created new cache in /home/testuser/DEVFEDORA/volsync/RESTICTESTS/CACHE
=== Starting forget ===
=== Done ===`

			reader := strings.NewReader(testLog)
			filteredLines, err := utils.FilterLogs(reader, testFilterFunc)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})

		Context("When MOVER_LOG_DEBUG is true, filterfunc should be ignored (log everything", func() {
			BeforeEach(func() {
				// Set env var to true
				os.Setenv(utils.MoverLogDebugEnvVar, "true")
			})
			AfterEach(func() {
				// Unset the env to set back to default
				os.Unsetenv(utils.MoverLogDebugEnvVar)
			})

			It("Should return all lines, ignoring the lineFilter", func() {
				reader := strings.NewReader(testLog)
				filteredLines, err := utils.FilterLogs(reader, testFilterFunc)
				Expect(err).NotTo(HaveOccurred())

				logger.Info("Filtered lines are", "filteredLines", filteredLines)
				Expect(filteredLines).To(Equal(testLog))
			})
		})
	})
})

var _ = Describe("Truncate string test", func() {
	It("Should truncate the beginning of the string", func() {
		s1 := "this is my test string\nSecond line here" // 39 bytes

		Expect(utils.TruncateString(s1, 0)).To(Equal(""))

		Expect(utils.TruncateString(s1, 1)).To(Equal("e"))
		Expect(utils.TruncateString(s1, 5)).To(Equal(" here"))

		Expect(utils.TruncateString(s1, 4000)).To(Equal(s1))
		Expect(utils.TruncateString(s1, 40)).To(Equal(s1))
		Expect(utils.TruncateString(s1, 39)).To(Equal(s1))

		Expect(utils.TruncateString(s1, 38)).To(Equal("his is my test string\nSecond line here"))
	})
})

var _ = Describe("Tail lines env var test", func() {
	It("Should test the default value for the tail logs env var", func() {
		tailLinesDefault := utils.GetMoverLogTailLines()
		Expect(tailLinesDefault).To(Equal(int64(-1)))
		Expect(tailLinesDefault < 0).To(BeTrue())
	})

	When("The env var is set to a value", func() {
		BeforeEach(func() {
			os.Setenv(utils.MoverLogTailLinesEnvVar, "123")
		})
		AfterEach(func() {
			os.Unsetenv(utils.MoverLogTailLinesEnvVar)
		})

		It("Should use the value provided and convert to int64", func() {
			tailLines := utils.GetMoverLogTailLines()
			Expect(tailLines).To(Equal(int64(123)))
		})
	})
})

func testFilterFunc(line string) *string {
	// Return all lines that start with "Created " or "created " or lines that have "=== * ==="
	var myRegex = regexp.MustCompile(`^\s*([cC]reated)\s.+|^\s*(===)\s.+(===)`)

	if myRegex.MatchString(line) {
		return &line
	}
	return nil
}

func createTestPodForJob(name, namespace, jobName string, desiredPhase corev1.PodPhase) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"job-name": jobName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "test",
				Command: []string{"/bin/fakemovercmd.sh"},
				Image:   "someimagepath:here",
			}},
		},
	}

	Expect(k8sClient.Create(ctx, pod)).To(Succeed())

	// Set the desired phase in the status (note that default is "Pending")
	pod.Status.Phase = desiredPhase
	Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

	// These tests are using the k8sClientSet (we init it with utils.InitPodLogsClient()) - as client-go is needed
	// to get pod logs. This client is caching by default, so make sure it has loaded the pod that was just created
	Eventually(func() bool {
		p, err := k8sClientSet.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		return p.Status.Phase == desiredPhase
	}, timeout, interval).Should(BeTrue())

	return pod
}
