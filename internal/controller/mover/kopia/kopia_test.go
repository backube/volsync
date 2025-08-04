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
	"flag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia", func() {
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())
	})

	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("Kopia mover builder", func() {
		It("should have correct name", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.Name()).To(Equal("kopia"))
		})

		It("should return version info", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())
			versionInfo := b.VersionInfo()
			Expect(versionInfo).To(ContainSubstring("Kopia container"))
		})

		It("should return nil mover when spec is nil", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rs",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "test",
				},
			}

			m, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).To(BeNil())
			Expect(m).To(BeNil())

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: ns.Name,
				},
			}

			m, err = b.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd, false)
			Expect(err).To(BeNil())
			Expect(m).To(BeNil())
		})

		It("should create mover from source", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rs",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "test",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-secret",
						ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
							CopyMethod: volsyncv1alpha1.CopyMethodSnapshot,
						},
					},
				},
				Status: &volsyncv1alpha1.ReplicationSourceStatus{},
			}

			m, err := b.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs, false)
			Expect(err).To(BeNil())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("kopia"))
		})

		It("should create mover from destination", func() {
			viper := viper.New()
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			b, err := newBuilder(viper, flags)
			Expect(err).NotTo(HaveOccurred())

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "kopia-secret",
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							Capacity: ptr.To(resource.MustParse("1Gi")),
						},
					},
				},
				Status: &volsyncv1alpha1.ReplicationDestinationStatus{},
			}

			m, err := b.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd, false)
			Expect(err).To(BeNil())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("kopia"))
		})
	})

	Context("Kopia log filter", func() {
		It("should filter kopia log lines", func() {
			// Test lines that should be included
			testLines := []string{
				"Snapshot k123456 created",
				"Uploaded 1234 bytes",
				"Restored 100 files",
				"Successfully completed",
				"Connected to repository",
				"Repository opened",
				"Maintenance completed",
				"ERROR: something went wrong",
				"FATAL: critical error",
				"kopia completed in 30s",
			}

			for _, line := range testLines {
				result := LogLineFilterSuccess(line)
				Expect(result).NotTo(BeNil())
				Expect(*result).To(Equal(line))
			}

			// Test lines that should be filtered out
			filteredLines := []string{
				"Random debug message",
				"Verbose internal details",
				"Unimportant status update",
			}

			for _, line := range filteredLines {
				result := LogLineFilterSuccess(line)
				Expect(result).To(BeNil())
			}
		})
	})
})
