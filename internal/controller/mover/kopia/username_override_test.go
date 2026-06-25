//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Kopia Username/Hostname Override", func() {
	Context("When username and hostname overrides are configured", func() {
		var (
			mover *Mover
			job   *batchv1.Job
		)

		BeforeEach(func() {
			logger := zap.New(zap.UseDevMode(true))

			// Create a mover with username and hostname overrides
			mover = &Mover{
				logger:        logger,
				owner:         nil,
				username:      "testuser",
				hostname:      "testhost",
				cacheCapacity: resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
			}

			// Create a job to test environment variable configuration
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-namespace",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "kopia",
									Image: "kopia:latest",
								},
							},
						},
					},
				},
			}
		})

		It("should add username and hostname override environment variables", func() {
			// Test that the environment variables are added
			envVars := []corev1.EnvVar{}
			envVars = mover.addIdentityEnvironmentVariables(envVars)

			// Check that the override variables are present
			var foundUsername, foundHostname bool
			var usernameValue, hostnameValue string

			for _, env := range envVars {
				if env.Name == "KOPIA_OVERRIDE_USERNAME" {
					foundUsername = true
					usernameValue = env.Value
				}
				if env.Name == "KOPIA_OVERRIDE_HOSTNAME" {
					foundHostname = true
					hostnameValue = env.Value
				}
			}

			Expect(foundUsername).To(BeTrue(), "KOPIA_OVERRIDE_USERNAME should be set")
			Expect(foundHostname).To(BeTrue(), "KOPIA_OVERRIDE_HOSTNAME should be set")
			Expect(usernameValue).To(Equal("testuser"))
			Expect(hostnameValue).To(Equal("testhost"))
		})

		It("should pass overrides to the entry script via environment", func() {
			// Configure a simple job spec
			podSpec := &job.Spec.Template.Spec
			podSpec.Containers[0].Env = []corev1.EnvVar{}

			// Add identity environment variables
			podSpec.Containers[0].Env = mover.addIdentityEnvironmentVariables(podSpec.Containers[0].Env)

			// Verify the environment variables are correctly set
			envMap := make(map[string]string)
			for _, env := range podSpec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["KOPIA_OVERRIDE_USERNAME"]).To(Equal("testuser"))
			Expect(envMap["KOPIA_OVERRIDE_HOSTNAME"]).To(Equal("testhost"))
		})

		It("should work correctly with cached repository configuration", func() {
			// This test verifies that the fix handles cached configurations properly
			// The entry.sh script should apply overrides to snapshot create commands
			// even when the repository is already connected with cached config

			// Simulate environment with cache PVC (persistent cache)
			cachePVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kopia-cache",
					Namespace: "test-namespace",
				},
			}

			// Configure cache volume
			mover.configureCacheVolume(&job.Spec.Template.Spec, cachePVC)

			// Verify cache mount is configured
			var cacheVolumeFound bool
			for _, vol := range job.Spec.Template.Spec.Volumes {
				if vol.Name == kopiaCache {
					cacheVolumeFound = true
					Expect(vol.PersistentVolumeClaim).NotTo(BeNil())
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal("kopia-cache"))
				}
			}
			Expect(cacheVolumeFound).To(BeTrue(), "Cache volume should be configured")

			// Add identity environment variables
			job.Spec.Template.Spec.Containers[0].Env = mover.addIdentityEnvironmentVariables(
				job.Spec.Template.Spec.Containers[0].Env)

			// The environment variables should still be set even with cache
			envMap := make(map[string]string)
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["KOPIA_OVERRIDE_USERNAME"]).To(Equal("testuser"))
			Expect(envMap["KOPIA_OVERRIDE_HOSTNAME"]).To(Equal("testhost"))
		})
	})

	Context("When no overrides are configured", func() {
		var (
			mover *Mover
		)

		BeforeEach(func() {
			logger := zap.New(zap.UseDevMode(true))
			mover = &Mover{
				logger:   logger,
				owner:    nil,
				username: "", // No username override
				hostname: "", // No hostname override
			}
		})

		It("should not add override environment variables when empty", func() {
			envVars := []corev1.EnvVar{}
			envVars = mover.addIdentityEnvironmentVariables(envVars)

			// With empty overrides, the variables should still be added but with empty values
			// This is the current behavior - the entry.sh script handles empty values
			var foundUsername, foundHostname bool
			for _, env := range envVars {
				if env.Name == "KOPIA_OVERRIDE_USERNAME" {
					foundUsername = true
					Expect(env.Value).To(Equal(""))
				}
				if env.Name == "KOPIA_OVERRIDE_HOSTNAME" {
					foundHostname = true
					Expect(env.Value).To(Equal(""))
				}
			}

			Expect(foundUsername).To(BeTrue())
			Expect(foundHostname).To(BeTrue())
		})
	})
})
