//go:build !disable_restic

/*
Copyright 2021 The VolSync authors.

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

package restic

import (
	"context"
	"flag"
	"os"
	"path"
	"strings"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	vsmover "github.com/backube/volsync/internal/controller/mover"
	"github.com/backube/volsync/internal/controller/utils"
)

const (
	timeout  = "30s"
	interval = "1s"
)

var _ = Describe("Restic retain policy", func() {
	Context("When a retain policy is omitted", func() {
		It("has forget options that keep only the last backup", func() {
			var policy *volsyncv1alpha1.ResticRetainPolicy = nil
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-last\\s+1\\s*$"))
		})
	})
	Context("When a retain policy is empty", func() {
		It("has forget options that keep only the last backup", func() {
			policy := &volsyncv1alpha1.ResticRetainPolicy{}
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-last\\s+1\\s*$"))
		})
	})
	Context("When a retain policy is specified", func() {
		It("has forget options that correspond", func() {
			one := int32(1)
			two := int32(2)
			three := int32(3)
			four := int32(4)
			five := int32(5)
			policy := &volsyncv1alpha1.ResticRetainPolicy{
				Hourly:  &five,
				Daily:   &four,
				Weekly:  &three,
				Monthly: &two,
				Yearly:  &one,
			}
			forget := generateForgetOptions(policy)
			Expect(forget).NotTo(MatchRegexp("--keep-last"))
			Expect(forget).NotTo(MatchRegexp("--within"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-hourly\\s+5(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-daily\\s+4(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-weekly\\s+3(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-monthly\\s+2(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-yearly\\s+1(\\s|$)"))
		})
		It("permits time-based retention", func() {
			duration := "5m3w1d"
			policy := &volsyncv1alpha1.ResticRetainPolicy{
				Within: &duration,
			}
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-within\\s+5m3w1d\\s*$"))
		})
	})
})

var _ = Describe("Restic unlock", func() {
	var m *Mover
	var owner *corev1.ConfigMap
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var start metav1.Time

	BeforeEach(func() {
		start = metav1.Now()
		// The underlying type of owner doesn't matter
		owner = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "name",
				Namespace:         "ns",
				CreationTimestamp: start,
			},
		}
		m = &Mover{
			logger:       logger,
			owner:        owner,
			sourceStatus: &volsyncv1alpha1.ReplicationSourceResticStatus{},
		}
	})

	When("Unlock is not set in the spec", func() {
		It("shouldUnlock() should return false", func() {
			Expect(m.shouldUnlock()).To(BeFalse())
		})

		When("the status has a lastUnlocked value set", func() {
			// If unlock is not set in the spec, we should not run unlock regardless of what is set in the status.
			// So checking here to make sure we don't blindly compare spec.restic.unlock with status.restic.lastUnlocked
			It("shouldUnlock() should still return false", func() {
				m.sourceStatus.LastUnlocked = "prev-unlock-value"
				Expect(m.shouldUnlock()).To(BeFalse())
			})
		})
	})
	When("Unlock set to a value", func() {
		BeforeEach(func() {
			m.unlock = "unlock-now"
		})
		When("No lastUnlock is set in status", func() {
			It("shouldUnlock() should return true", func() {
				Expect(m.shouldUnlock()).To(BeTrue())
			})
		})
		When("lastUnlock is set to unlock in status", func() {
			It("shouldUnlock() should return false", func() {
				m.sourceStatus.LastUnlocked = m.unlock // Set status to unlock value
				Expect(m.shouldUnlock()).To(BeFalse())
			})
		})
		When("lastUnlock is set to different value from unlock in status", func() {
			It("shouldUnlock() should return true", func() {
				m.sourceStatus.LastUnlocked = "some-new-value"
				Expect(m.shouldUnlock()).To(BeTrue())
			})
		})

	})

})

var _ = Describe("Restic prune policy", func() {
	var m *Mover
	var owner *corev1.ConfigMap
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var start metav1.Time

	BeforeEach(func() {
		start = metav1.Now()
		// The underlying type of owner doesn't matter
		owner = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "name",
				Namespace:         "ns",
				CreationTimestamp: start,
			},
		}
		m = &Mover{
			logger:        logger,
			owner:         owner,
			pruneInterval: nil,
			sourceStatus:  &volsyncv1alpha1.ReplicationSourceResticStatus{},
		}
	})
	When("Interval is omitted it defaults to 1 week", func() {
		const week = time.Hour * 24 * 7
		It("waits from creation", func() {
			current := start.Add(time.Minute)
			Expect(m).NotTo(BeNil())
			Expect(m.shouldPrune(current)).To(BeFalse())
			afterWeek := start.Add(week + time.Minute)
			Expect(m.shouldPrune(afterWeek)).To(BeTrue())
		})
		It("uses the last pruned time", func() {
			lastPruned := start.Add(time.Hour)
			m.sourceStatus.LastPruned = &metav1.Time{Time: lastPruned}

			Expect(m.shouldPrune(start.Time)).To(BeFalse())
			Expect(m.shouldPrune(lastPruned.Add(time.Minute))).To(BeFalse())
			Expect(m.shouldPrune(lastPruned.Add(week + time.Minute))).To(BeTrue())
		})
	})
	When("Interval is provided", func() {
		const day = 24 * time.Hour
		It("waits from creation", func() {
			interval := int32(3) // 3 days
			m.pruneInterval = &interval

			current := start.Add(time.Minute)
			Expect(m).NotTo(BeNil())
			Expect(m.shouldPrune(current)).To(BeFalse())
			after := start.Add(time.Duration(interval)*day + time.Minute)
			Expect(m.shouldPrune(after)).To(BeTrue())
		})
		It("uses the last pruned time", func() {
			lastPruned := start.Add(time.Hour)
			m.sourceStatus.LastPruned = &metav1.Time{Time: lastPruned}
			interval := int32(21) // 21 days
			m.pruneInterval = &interval

			Expect(m.shouldPrune(start.Time)).To(BeFalse())
			Expect(m.shouldPrune(lastPruned.Add(time.Minute))).To(BeFalse())
			Expect(m.shouldPrune(lastPruned.Add(time.Duration(interval)*day + time.Minute))).To(BeTrue())
		})
	})
})

var _ = Describe("Restic properly registers", func() {
	When("Restic's registration function is called", func() {
		BeforeEach(func() {
			Expect(Register()).To(Succeed())
		})

		It("is added to the mover catalog", func() {
			found := false
			for _, v := range vsmover.Catalog {
				if _, ok := v.(*Builder); ok {
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})

var _ = Describe("Restic inits flags and env vars", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("Restic builder inits flags", func() {
		var builderForInitTests *Builder
		var testPflagSet *pflag.FlagSet
		BeforeEach(func() {
			os.Unsetenv(resticContainerImageEnvVar)

			// Instantiate new viper instance and flagSet just for this test
			testViper := viper.New()
			testFlagSet := flag.NewFlagSet("testflagsetrestic", flag.ExitOnError)

			// New Builder for this test - use testViper and testFlagSet so we can modify
			// flags for these tests without modifying global flags and potentially affecting other tests
			var err error
			builderForInitTests, err = newBuilder(testViper, testFlagSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(builderForInitTests).NotTo(BeNil())

			// code here (see main.go) for viper to bind cmd line flags (including those
			// defined in the mover Register() func)
			// Bind viper to a new set of flags so each of these tests can get their own
			testPflagSet = pflag.NewFlagSet("testpflagsetrestic", pflag.ExitOnError)
			testPflagSet.AddGoFlagSet(testFlagSet)
			Expect(testViper.BindPFlags(testPflagSet)).To(Succeed())
		})

		AfterEach(func() {
			os.Unsetenv(resticContainerImageEnvVar)
		})

		JustBeforeEach(func() {
			// Common checks - make sure if we instantiate a source/dest mover, it uses the container image that
			// was picked up by flags/command line etc from the builder
			var err error

			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testrscr",
					Namespace: "testing",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Restic: &volsyncv1alpha1.ReplicationSourceResticSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationSourceStatus{}, // Controller sets status to non-nil
			}
			sourceMover, err := builderForInitTests.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(err).NotTo(HaveOccurred())
			Expect(sourceMover).NotTo(BeNil())
			sourceResticMover, _ := sourceMover.(*Mover)
			Expect(sourceResticMover.containerImage).To(Equal(builderForInitTests.getResticContainerImage()))

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: "testing",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Trigger: &volsyncv1alpha1.ReplicationDestinationTriggerSpec{},
					Restic:  &volsyncv1alpha1.ReplicationDestinationResticSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationDestinationStatus{}, // Controller sets status to non-nil
			}
			destMover, err := builderForInitTests.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(err).NotTo(HaveOccurred())
			Expect(destMover).NotTo(BeNil())
			destResticMover, _ := destMover.(*Mover)
			Expect(destResticMover.containerImage).To(Equal(builderForInitTests.getResticContainerImage()))
		})

		Context("When no command line flag or ENV var is specified", func() {
			It("Should use the default restic container image", func() {
				Expect(builderForInitTests.getResticContainerImage()).To(Equal(defaultResticContainerImage))
			})
		})

		Context("When restic container image command line flag is specified", func() {
			const cmdLineOverrideImageName = "test-restic-image-name:cmdlineoverride"
			BeforeEach(func() {
				// Manually set the value of the command line flag
				Expect(testPflagSet.Set("restic-container-image", cmdLineOverrideImageName)).To(Succeed())
			})
			It("Should use the restic container image set by the cmd line flag", func() {
				Expect(builderForInitTests.getResticContainerImage()).To(Equal(cmdLineOverrideImageName))
			})

			Context("And env var is set", func() {
				const envVarOverrideShouldBeIgnored = "test-restic-image-name:donotuseme"
				BeforeEach(func() {
					os.Setenv(resticContainerImageEnvVar, envVarOverrideShouldBeIgnored)
				})
				It("Should still use the cmd line flag instead of the env var", func() {
					Expect(builderForInitTests.getResticContainerImage()).To(Equal(cmdLineOverrideImageName))
				})
			})
		})

		Context("When resticc container image cmd line flag is not set and env var is", func() {
			const envVarOverrideImageName = "test-restic-image-name:setbyenvvar"
			BeforeEach(func() {
				os.Setenv(resticContainerImageEnvVar, envVarOverrideImageName)
			})
			It("Should use the value from the env var", func() {
				Expect(builderForInitTests.getResticContainerImage()).To(Equal(envVarOverrideImageName))
			})
		})
	})
})

var _ = Describe("Restic ignores other movers", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("An RS isn't for restic", func() {
		It("is ignored", func() {
			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr",
					Namespace: "blah",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Restic: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
	When("An RD isn't for restic", func() {
		It("is ignored", func() {
			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x",
					Namespace: "y",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Restic: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Restic as a source", func() {
	var ctx = context.TODO()
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rs *volsyncv1alpha1.ReplicationSource
	var sPVC *corev1.PersistentVolumeClaim
	var mover *Mover
	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())

		sc := "spvcsc"
		sPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s",
				Namespace: ns.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": resource.MustParse("7Gi"),
					},
				},
				StorageClassName: &sc,
			},
		}
		Expect(k8sClient.Create(ctx, sPVC)).To(Succeed())

		// Scaffold ReplicationSource
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rs",
				Namespace: ns.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: sPVC.Name,
				Trigger:   &volsyncv1alpha1.ReplicationSourceTriggerSpec{},
				Restic:    &volsyncv1alpha1.ReplicationSourceResticSpec{},
				Paused:    false,
			},
		}
	})
	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, rs)).To(Succeed())
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})
	When("used as source", func() {
		JustBeforeEach(func() {
			// Controller sets status to non-nil
			rs.Status = &volsyncv1alpha1.ReplicationSourceStatus{}
			// Instantiate a restic mover for the tests
			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("validate repo secret", func() {
			var repo *corev1.Secret
			BeforeEach(func() {
				repo = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "x",
						Namespace: ns.Name,
					},
				}
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
				rs.Spec.Restic.Repository = repo.Name
			})
			It("validates that the required keys are present", func() {
				testdata := []struct {
					keys []string
					ok   bool
				}{
					{keys: []string{"RESTIC_REPOSITORY", "RESTIC_PASSWORD"}, ok: true},
					{keys: []string{"RESTIC_REPOSITORY"}, ok: false},
					{keys: []string{"RESTIC_PASSWORD"}, ok: false},
					{keys: []string{"another_key", "RESTIC_REPOSITORY", "RESTIC_PASSWORD"}, ok: true},
					{keys: []string{}, ok: false},
				}
				for _, td := range testdata {
					repo.Data = map[string][]byte{}
					for _, k := range td.keys {
						repo.Data[k] = []byte("HELLO")
					}
					Expect(k8sClient.Update(ctx, repo)).To(Succeed())
					s, e := mover.validateRepository(ctx)
					if td.ok {
						Expect(s).NotTo(BeNil())
						Expect(e).NotTo(HaveOccurred())
					} else {
						Expect(s).To(BeNil())
						Expect(e).To(HaveOccurred())
					}
				}
			})
		})

		Context("Restic cache is created correctly", func() {
			var dataPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				dataPVC = sPVC
			})

			When("default (no cleanupCachePVC set)", func() {
				It("Should not have the cleanup label", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
			When("cleanupCachePVC set to true", func() {
				It("Should not have the cleanup label", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, true /* cleanup */)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Labels).To(HaveKey("volsync.backube/cleanup"))
				})
			})

			When("no capacity is specified", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CacheCapacity = nil
				})
				It("is 1Gi is size", func() {
					oneGB := resource.MustParse("1Gi")
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(*cache.Spec.Resources.Requests.Storage()).To(Equal(oneGB))
				})
			})
			When("capacity is set", func() {
				theSize := resource.MustParse("10Gi")
				BeforeEach(func() {
					rs.Spec.Restic.CacheCapacity = &theSize
				})
				It("uses the specified size", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(*cache.Spec.Resources.Requests.Storage()).To(Equal(theSize))
				})
			})

			When("no accessMode is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.AccessModes = nil
					rs.Spec.Restic.CacheAccessModes = nil
				})
				It("matches the source pvc", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal(dataPVC.Spec.AccessModes))
				})
			})
			When("options accessMode is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
					rs.Spec.Restic.CacheAccessModes = nil
				})
				It("matches the specified option", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}))
				})
			})
			When("cache accessMode is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
					rs.Spec.Restic.CacheAccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
				})
				It("uses the cache-specific mode", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal([]corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}))
				})
			})

			When("no storageClass is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.StorageClassName = nil
					rs.Spec.Restic.CacheStorageClassName = nil
				})
				It("uses the cluster default", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.StorageClassName).To(BeNil())
				})
			})
			When("storageClass option is set", func() {
				option := "option"
				BeforeEach(func() {
					rs.Spec.Restic.StorageClassName = &option
					rs.Spec.Restic.CacheStorageClassName = nil
				})
				It("matches the option SC", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(*cache.Spec.StorageClassName).To(Equal(option))
				})
			})
			When("the cache storageClass is set", func() {
				option := "option"
				cachesc := "cachesc"
				BeforeEach(func() {
					rs.Spec.Restic.StorageClassName = &option
					rs.Spec.Restic.CacheStorageClassName = &cachesc
				})
				It("matches the cache SC", func() {
					cache, err := mover.ensureCache(ctx, dataPVC, false)
					Expect(err).ToNot(HaveOccurred())
					Expect(*cache.Spec.StorageClassName).To(Equal(cachesc))
				})
			})

			Context("old cache name migration", func() {
				var oldCache *corev1.PersistentVolumeClaim
				When("The cache PVC with the old name exists", func() {
					BeforeEach(func() {
						sc := "some-storage"
						oldCache = &corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{
								Name:      vsmover.VolSyncPrefix + rs.GetName() + "-cache",
								Namespace: rs.GetNamespace(),
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{
										"storage": resource.MustParse("1Gi"),
									},
								},
								StorageClassName: &sc,
							},
						}
					})

					JustBeforeEach(func() {
						Expect(k8sClient.Create(ctx, oldCache)).To(Succeed())
					})

					When("The old cache pvc is not owned by this VolSync RS", func() {
						It("Should not delete the old cache PVC and still create the new one", func() {
							cache, err := mover.ensureCache(ctx, dataPVC, false)
							Expect(err).ToNot(HaveOccurred())
							Expect(cache.GetName()).To(ContainSubstring("-cache"))
							Expect(cache.GetName()).To(ContainSubstring("-src-"))

							// Check that old cache pvc is still there
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(oldCache), oldCache)).To(Succeed())
							Expect(oldCache.GetDeletionTimestamp().IsZero()).To(BeTrue()) // Not marked for deletion
						})
					})

					When("The old cache PVC was owned by this VolSync RS", func() {
						JustBeforeEach(func() {
							// Need to do this after the RS has been created
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(oldCache), oldCache)).To(Succeed())
							Expect(ctrl.SetControllerReference(rs, oldCache, k8sClient.Scheme())).To(Succeed())
							Expect(k8sClient.Update(ctx, oldCache)).To(Succeed())
						})

						It("Should delete the old cache pvc and still create the new one", func() {
							cache, err := mover.ensureCache(ctx, dataPVC, false)
							Expect(err).ToNot(HaveOccurred())
							Expect(cache.GetName()).To(ContainSubstring("-cache"))
							Expect(cache.GetName()).To(ContainSubstring("-src-"))

							// Check that old cache pvc was deleted
							err = k8sClient.Get(ctx, client.ObjectKeyFromObject(oldCache), oldCache)
							if err == nil {
								// Should be marked for deletion
								Expect(oldCache.GetDeletionTimestamp().IsZero()).To(BeFalse())
							} else {
								Expect(kerrors.IsNotFound(err)).To(BeTrue())
							}

							// Should be able to reconcile again
							_, err = mover.ensureCache(ctx, dataPVC, false)
							Expect(err).ToNot(HaveOccurred())
						})
					})
				})
			})
		})

		Context("Source volume is handled properly", func() {
			When("CopyMethod is None", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CopyMethod = volsyncv1alpha1.CopyMethodNone
				})
				It("the source is used as the data PVC", func() {
					dataPVC, err := mover.ensureSourcePVC(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(dataPVC.Name).To(Equal(sPVC.Name))
					// It's not owned by the CR
					Expect(dataPVC.OwnerReferences).To(BeEmpty())
					// It won't be cleaned up at the end of the transfer
					Expect(dataPVC.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
			When("CopyMethod is Direct", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CopyMethod = volsyncv1alpha1.CopyMethodDirect
				})
				It("the source is used as the data PVC", func() {
					dataPVC, err := mover.ensureSourcePVC(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(dataPVC.Name).To(Equal(sPVC.Name))
					// It's not owned by the CR
					Expect(dataPVC.OwnerReferences).To(BeEmpty())
					// It won't be cleaned up at the end of the transfer
					Expect(dataPVC.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
			When("CopyMethod is Clone", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CopyMethod = volsyncv1alpha1.CopyMethodClone
				})
				It("the source is NOT used as the data PVC", func() {
					dataPVC, err := mover.ensureSourcePVC(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(dataPVC.Name).NotTo(Equal(sPVC.Name))
					// It's owned by the CR
					Expect(dataPVC.OwnerReferences).NotTo(BeEmpty())
					// It will be cleaned up at the end of the transfer
					Expect(dataPVC.Labels).To(HaveKey("volsync.backube/cleanup"))
				})
				When("the use-copy-trigger annotation exists on the source (data) PVC", func() {
					BeforeEach(func() {
						sPVC.Annotations = map[string]string{
							volsyncv1alpha1.UseCopyTriggerAnnotation: "yes", // Any value is ok
						}
						Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())
					})
					JustBeforeEach(func() {
						dataPVC, err := mover.ensureSourcePVC(ctx)
						Expect(err).NotTo(HaveOccurred())
						// VolSync should not create the clone yet (Waiting on user-supplied copy-trigger)
						Expect(dataPVC).To(BeNil())

						// re-load sPVC to see that volsync has added latest-copy-trigger annotation
						// k8sClient is direct in this test, so no need for Eventually()
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
						latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
						Expect(ok).To(BeTrue())
						Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueWaitingForTrigger))

						// Also should have a waiting-since timestamp annotation
						waitingSince, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
						Expect(ok).To(BeTrue())

						waitingSinceTime, err := time.Parse(time.RFC3339, waitingSince)
						Expect(err).NotTo(HaveOccurred())
						Expect(waitingSinceTime.Before(time.Now().Add(1 * time.Second)))
					})
					It("Should wait before creating the clone and set latest-copy-status to WaitingForTrigger", func() {
						// Tests are in JustBeforeEach above
						// Let's also confirm that no latest-status-trigger has been set yet
						_, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
						Expect(ok).To(BeFalse())
					})
					When("The trigger is not updated in > 10 mins", func() {
						JustBeforeEach(func() {
							// Fake out long waiting time by manually setting the latest-copy-waiting-since to an old value
							oldTime := time.Now().Add(-15 * time.Minute) // Subtract 15 mins
							sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation] = oldTime.UTC().Format(time.RFC3339)
							Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

							// Now run another ensureSourcePVC/reconcile
							dataPVC, err := mover.ensureSourcePVC(ctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(dataPVC).To(BeNil())
							// Err is not returned
							// However the latestMoverStatus should be updated to show the error
						})
						It("should update latestMoverStatus with an error", func() {
							Expect(mover.latestMoverStatus).NotTo(BeNil())
							Expect(mover.latestMoverStatus.Result).To(Equal(volsyncv1alpha1.MoverResultFailed))
							Expect(mover.latestMoverStatus.Logs).To(ContainSubstring("Timed out waiting for copy-trigger"))
						})
					})

					When("The user triggers the clone to proceed via copy-trigger annotation", func() {
						var dataPVC *corev1.PersistentVolumeClaim
						JustBeforeEach(func() {
							// set a copy-trigger annotation
							sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation] = "first-t"
							Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

							// Now run another ensureSourcePVC/reconcile
							var err error
							dataPVC, err = mover.ensureSourcePVC(ctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(dataPVC).NotTo(BeNil()) // Should have proceeded to create the clone PVC

							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
							latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
							Expect(ok).To(BeTrue())
							Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueInProgress))

							// waiting-since timestamp annotation should now be removed
							_, ok = sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
							Expect(ok).To(BeFalse())
						})
						It("Should proceed with the clone and update latest-copy-status accordingly", func() {
							// Tests are in JustBeforeEach above
							// Let's also confirm that no latest-status-trigger has been set yet
							// since the clone has not gone into ClaimBound state
							_, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
							Expect(ok).To(BeFalse())
						})
						When("The clone goes to ClaimBound", func() {
							JustBeforeEach(func() {
								// Manually update clone to fake ClaimBound status
								dataPVC.Status.Phase = corev1.ClaimBound
								Expect(k8sClient.Status().Update(ctx, dataPVC)).To(Succeed())

								// Now run another ensureSourcePVC/reconcile
								var err error
								dataPVC, err = mover.ensureSourcePVC(ctx)
								Expect(err).NotTo(HaveOccurred())
								Expect(dataPVC).NotTo(BeNil())

								// re-load sourcePVC to see annotation updates
								Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())

								latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
								Expect(ok).To(BeTrue())
								Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueCompleted))

								latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
								Expect(ok).To(BeTrue())
								Expect(latestCopyTrigger).To(Equal("first-t"))
							})
							It("Should update the latest-copy-trigger and status annotations", func() {
								// See checks in JustBeforeEach
								latestCopyTrigger := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
								userCopyTrigger := sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation]
								Expect(latestCopyTrigger).To(Equal(userCopyTrigger))
							})
							When("Another clone needs to be created (another sync)", func() {
								JustBeforeEach(func() {
									// remove the clone to simulate sync done, need to create for
									// the next sync
									Expect(k8sClient.Delete(ctx, dataPVC)).To(Succeed())

									// Make sure the PVC is actually deleted or we will not be
									// able to simulate a new sync (new clone required)
									dataPVCReloaded := &corev1.PersistentVolumeClaim{}
									err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dataPVC), dataPVCReloaded)
									if err != nil {
										Expect(kerrors.IsNotFound(err)).To(BeTrue())
									} else {
										// clone pvc still exists - clear out finalizers to let it delete
										dataPVCReloaded.Finalizers = []string{}
										Expect(k8sClient.Update(ctx, dataPVCReloaded)).To(Succeed())
									}

									Eventually(func() bool {
										// Re-load until it is not found anymore
										err = k8sClient.Get(ctx, client.ObjectKeyFromObject(dataPVCReloaded), dataPVCReloaded)
										return err != nil && kerrors.IsNotFound(err)
									}, timeout, interval).Should(BeTrue())

									// Now run another ensureSourcePVC/reconcile
									dataPVCClone2, err := mover.ensureSourcePVC(ctx)
									Expect(err).NotTo(HaveOccurred())
									Expect(dataPVCClone2).To(BeNil())

									// re-load sPVC to see that volsync has added latest-copy-trigger annotation
									// k8sClient is direct in this test, so no need for Eventually()
									Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
									latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
									Expect(ok).To(BeTrue())
									Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueWaitingForTrigger))

									// Also should have a waiting-since timestamp annotation
									waitingSince, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
									Expect(ok).To(BeTrue())

									waitingSinceTime, err := time.Parse(time.RFC3339, waitingSince)
									Expect(err).NotTo(HaveOccurred())
									Expect(waitingSinceTime.Before(time.Now().Add(1 * time.Second)))
								})
								It("Should again wait for the copy-trigger to be updated to a new value", func() {
									// the latestcopytrigger should still be from the previous sync
									latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
									Expect(ok).To(BeTrue())
									Expect(latestCopyTrigger).To(Equal("first-t"))
								})
								When("The user updates the copy-trigger to a new value", func() {
									var dataPVCClone2 *corev1.PersistentVolumeClaim

									JustBeforeEach(func() {
										// update the copy-trigger annotation
										sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation] = "second-t"
										Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

										// Now run another ensureSourcePVC/reconcile
										var err error
										dataPVCClone2, err = mover.ensureSourcePVC(ctx)
										Expect(err).NotTo(HaveOccurred())
										Expect(dataPVCClone2).NotTo(BeNil()) // Should have proceeded to create the clone PVC

										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
										latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
										Expect(ok).To(BeTrue())
										Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueInProgress))

										// waiting-since timestamp annotation should now be removed
										_, ok = sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
										Expect(ok).To(BeFalse())
									})
									It("Should proceed with the clone and update latest-copy-status accordingly", func() {
										// Tests are in JustBeforeEach above
										// Let's also confirm that latest-status-trigger still has the previous trigger
										latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
										Expect(ok).To(BeTrue())
										Expect(latestCopyTrigger).To(Equal("first-t"))
									})
									When("The clone goes to ClaimBound", func() {
										JustBeforeEach(func() {
											// Manually update clone to fake ClaimBound status
											dataPVCClone2.Status.Phase = corev1.ClaimBound
											Expect(k8sClient.Status().Update(ctx, dataPVCClone2)).To(Succeed())

											// Now run another ensureSourcePVC/reconcile
											var err error
											dataPVCClone2, err = mover.ensureSourcePVC(ctx)
											Expect(err).NotTo(HaveOccurred())
											Expect(dataPVCClone2).NotTo(BeNil())

											// re-load sourcePVC to see annotation updates
											Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())

											latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
											Expect(ok).To(BeTrue())
											Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueCompleted))

											latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
											Expect(ok).To(BeTrue())
											Expect(latestCopyTrigger).To(Equal("second-t"))
										})
										It("Should update the latest-copy-trigger and status annotations", func() {
											// See checks in JustBeforeEach
											latestCopyTrigger := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
											userCopyTrigger := sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation]
											Expect(latestCopyTrigger).To(Equal(userCopyTrigger))
										})
									})
								})
							})
						})
					})
				})
			})

			When("CopyMethod is Snapshot", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
				})
				It("the source is not used as data pvc, snapshot is created", func() {
					_, err := mover.ensureSourcePVC(ctx)
					Expect(err).ToNot(HaveOccurred())

					// Snapshot should have been created
					snapshots := &snapv1.VolumeSnapshotList{}
					Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))).To(Succeed())
					Expect(len(snapshots.Items)).To(Equal(1))

					// update the VS name
					snapshot := snapshots.Items[0]
					foo := "dummysourcesnapshot"
					snapshot.Status = &snapv1.VolumeSnapshotStatus{
						BoundVolumeSnapshotContentName: &foo,
					}
					Expect(k8sClient.Status().Update(ctx, &snapshot)).To(Succeed())

					var dataPVC *corev1.PersistentVolumeClaim
					Eventually(func() bool {
						dataPVC, err = mover.ensureSourcePVC(ctx)
						return dataPVC != nil && err == nil
					}, timeout, interval).Should(BeTrue())
					Expect(err).ToNot(HaveOccurred())
					Expect(dataPVC.Name).NotTo(Equal(sPVC.Name))
					// It's owned by the CR
					Expect(dataPVC.OwnerReferences).NotTo(BeEmpty())
					// It will be cleaned up at the end of the transfer
					Expect(dataPVC.Labels).To(HaveKey("volsync.backube/cleanup"))
					// It should have datasource which is a snapshot
					Expect(dataPVC.Spec.DataSource.Name).To(Equal(snapshot.Name))
					Expect(dataPVC.Spec.DataSource.Kind).To(Equal("VolumeSnapshot"))
				})

				//nolint:dupl
				When("the use-copy-trigger annotation exists on the source (data) PVC", func() {
					BeforeEach(func() {
						sPVC.Annotations = map[string]string{
							volsyncv1alpha1.UseCopyTriggerAnnotation: "", // Any value is ok
						}
						Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())
					})
					JustBeforeEach(func() {
						dataPVC, err := mover.ensureSourcePVC(ctx)
						Expect(err).NotTo(HaveOccurred())
						// VolSync should not create the snap or pvc-from-snap yet (Waiting on user-supplied copy-trigger)
						Expect(dataPVC).To(BeNil())

						snapshots := &snapv1.VolumeSnapshotList{}
						Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))).To(Succeed())
						Expect(len(snapshots.Items)).To(Equal(0))

						// re-load sPVC to see that volsync has added latest-copy-trigger annotation
						// k8sClient is direct in this test, so no need for Eventually()
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
						latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
						Expect(ok).To(BeTrue())
						Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueWaitingForTrigger))

						// Also should have a waiting-since timestamp annotation
						waitingSince, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
						Expect(ok).To(BeTrue())

						waitingSinceTime, err := time.Parse(time.RFC3339, waitingSince)
						Expect(err).NotTo(HaveOccurred())
						Expect(waitingSinceTime.Before(time.Now().Add(1 * time.Second)))
					})
					It("Should wait before creating the snapshot and set latest-copy-status to WaitingForTrigger", func() {
						// Tests are in JustBeforeEach above
						// Let's also confirm that no latest-status-trigger has been set yet
						_, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
						Expect(ok).To(BeFalse())
					})
					When("The trigger is not updated in > 10 mins", func() {
						JustBeforeEach(func() {
							// Fake out long waiting time by manually setting the latest-copy-waiting-since to an old value
							oldTime := time.Now().Add(-15 * time.Minute) // Subtract 15 mins
							sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation] = oldTime.UTC().Format(time.RFC3339)
							Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

							// Now run another ensureSourcePVC/reconcile
							dataPVC, err := mover.ensureSourcePVC(ctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(dataPVC).To(BeNil())
							// Err is not returned
							// However the latestMoverStatus should be updated to show the error
						})
						It("should update latestMoverStatus with an error", func() {
							Expect(mover.latestMoverStatus).NotTo(BeNil())
							Expect(mover.latestMoverStatus.Result).To(Equal(volsyncv1alpha1.MoverResultFailed))
							Expect(mover.latestMoverStatus.Logs).To(ContainSubstring("Timed out waiting for copy-trigger"))
						})
					})
					When("The user triggers the snapshot to proceed via copy-trigger annotation", func() {
						var firstSyncDataPVC *corev1.PersistentVolumeClaim
						var firstSyncSnapshot *snapv1.VolumeSnapshot
						JustBeforeEach(func() {
							// set a copy-trigger annotation
							sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation] = "first-t"
							Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

							// Now run another ensureSourcePVC/reconcile
							var err error
							firstSyncDataPVC, err = mover.ensureSourcePVC(ctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(firstSyncDataPVC).To(BeNil()) // Will not create the PVC as snapshot isn't ready yet

							// Snapshot however should have been created
							snapshots := &snapv1.VolumeSnapshotList{}
							Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))).To(Succeed())
							Expect(len(snapshots.Items)).To(Equal(1))

							firstSyncSnapshot = &snapshots.Items[0]

							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
							latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
							Expect(ok).To(BeTrue())
							Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueInProgress))

							// waiting-since timestamp annotation should now be removed
							_, ok = sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
							Expect(ok).To(BeFalse())
						})
						It("Should proceed with the snapshot and update latest-copy-status accordingly", func() {
							// Tests are in JustBeforeEach above
							// Let's also confirm that no latest-status-trigger has been set yet
							// since the clone has not gone into ClaimBound state
							_, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
							Expect(ok).To(BeFalse())
						})
						When("The snapshot gets boundVolumeSnapshotContentName (Snapshot is ready)", func() {
							JustBeforeEach(func() {
								// update the volumesnapshot to set BoundVolumeSnapshotContentName
								foo := "dummysourcesnapshot"
								firstSyncSnapshot.Status = &snapv1.VolumeSnapshotStatus{
									BoundVolumeSnapshotContentName: &foo,
								}
								Expect(k8sClient.Status().Update(ctx, firstSyncSnapshot)).To(Succeed())

								// Now run another ensureSourcePVC/reconcile
								var err error
								firstSyncDataPVC, err = mover.ensureSourcePVC(ctx)
								Expect(err).NotTo(HaveOccurred())
								Expect(firstSyncDataPVC).NotTo(BeNil())

								// re-load sourcePVC to see annotation updates
								Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())

								latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
								Expect(ok).To(BeTrue())
								Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueCompleted))

								latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
								Expect(ok).To(BeTrue())
								Expect(latestCopyTrigger).To(Equal("first-t"))
							})
							It("Should update the latest-copy-trigger and status annotations", func() {
								// See checks in JustBeforeEach
								latestCopyTrigger := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
								userCopyTrigger := sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation]
								Expect(latestCopyTrigger).To(Equal(userCopyTrigger))
							})
							When("Another snapshot needs to be created (another sync)", func() {
								JustBeforeEach(func() {
									// remove the pvc-from-snap and snapshot to simulate sync done, need to create for
									// the next sync
									Expect(k8sClient.Delete(ctx, firstSyncDataPVC)).To(Succeed())

									// Make sure the PVC is actually deleted or we will not be
									// able to simulate a new sync (new clone required)
									dataPVCReloaded := &corev1.PersistentVolumeClaim{}
									err := k8sClient.Get(ctx, client.ObjectKeyFromObject(firstSyncDataPVC), dataPVCReloaded)
									if err != nil {
										Expect(kerrors.IsNotFound(err)).To(BeTrue())
									} else {
										// pvc from snap still exists - clear out finalizers to let it delete
										dataPVCReloaded.Finalizers = []string{}
										Expect(k8sClient.Update(ctx, dataPVCReloaded)).To(Succeed())
									}
									Eventually(func() bool {
										// Re-load until it is not found anymore
										err = k8sClient.Get(ctx, client.ObjectKeyFromObject(dataPVCReloaded), dataPVCReloaded)
										return err != nil && kerrors.IsNotFound(err)
									}, timeout, interval).Should(BeTrue())

									// Now remove the snapshot too
									Expect(k8sClient.Delete(ctx, firstSyncSnapshot)).To(Succeed())
									snapshotReloaded := &snapv1.VolumeSnapshot{}
									err = k8sClient.Get(ctx, client.ObjectKeyFromObject(firstSyncSnapshot), snapshotReloaded)
									Expect(err).To(HaveOccurred())
									Expect(kerrors.IsNotFound(err))

									// Now run another ensureSourcePVC/reconcile
									dataPVCSnap2, err := mover.ensureSourcePVC(ctx)
									Expect(err).NotTo(HaveOccurred())
									Expect(dataPVCSnap2).To(BeNil())

									// No new snap should be created yet since we're waiting on the copy-trigger
									snapshots := &snapv1.VolumeSnapshotList{}
									Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))).To(Succeed())
									Expect(len(snapshots.Items)).To(Equal(0))

									// re-load sPVC to see that volsync has added latest-copy-trigger annotation
									// k8sClient is direct in this test, so no need for Eventually()
									Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
									latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
									Expect(ok).To(BeTrue())
									Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueWaitingForTrigger))

									// Also should have a waiting-since timestamp annotation
									waitingSince, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
									Expect(ok).To(BeTrue())

									waitingSinceTime, err := time.Parse(time.RFC3339, waitingSince)
									Expect(err).NotTo(HaveOccurred())
									Expect(waitingSinceTime.Before(time.Now().Add(1 * time.Second)))
								})
								It("Should again wait for the copy-trigger to be updated to a new value", func() {
									// the latestcopytrigger should still be from the previous sync
									latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
									Expect(ok).To(BeTrue())
									Expect(latestCopyTrigger).To(Equal("first-t"))
								})
								When("The user updates the copy-trigger to a new value", func() {
									var secondDataPVC *corev1.PersistentVolumeClaim
									var secondSyncSnapshot *snapv1.VolumeSnapshot

									JustBeforeEach(func() {
										// update the copy-trigger annotation
										sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation] = "second-t"
										Expect(k8sClient.Update(ctx, sPVC)).To(Succeed())

										// Now run another ensureSourcePVC/reconcile
										var err error
										secondDataPVC, err = mover.ensureSourcePVC(ctx)
										Expect(err).NotTo(HaveOccurred())
										Expect(secondDataPVC).To(BeNil()) // No PVC should exists yet since snap is not ready

										// Snapshot however should have been created
										snapshots := &snapv1.VolumeSnapshotList{}
										Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))).To(Succeed())
										Expect(len(snapshots.Items)).To(Equal(1))

										secondSyncSnapshot = &snapshots.Items[0]

										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())
										latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
										Expect(ok).To(BeTrue())
										Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueInProgress))

										// waiting-since timestamp annotation should now be removed
										_, ok = sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerWaitingSinceAnnotation]
										Expect(ok).To(BeFalse())
									})
									It("Should proceed with the snap and update latest-copy-status accordingly", func() {
										// Tests are in JustBeforeEach above
										// Let's also confirm that latest-status-trigger still has the previous trigger
										latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
										Expect(ok).To(BeTrue())
										Expect(latestCopyTrigger).To(Equal("first-t"))
									})
									When("The snapshot gets boundVolumeSnapshotContentName (Snapshot is ready)", func() {
										JustBeforeEach(func() {
											// update the volumesnapshot to set BoundVolumeSnapshotContentName
											foo := "dummysourcesnapshot2"
											secondSyncSnapshot.Status = &snapv1.VolumeSnapshotStatus{
												BoundVolumeSnapshotContentName: &foo,
											}
											Expect(k8sClient.Status().Update(ctx, secondSyncSnapshot)).To(Succeed())

											// Now run another ensureSourcePVC/reconcile
											var err error
											secondDataPVC, err = mover.ensureSourcePVC(ctx)
											Expect(err).NotTo(HaveOccurred())
											Expect(secondDataPVC).NotTo(BeNil())

											// re-load sourcePVC to see annotation updates
											Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), sPVC)).To(Succeed())

											latestCopyStatus, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyStatusAnnotation]
											Expect(ok).To(BeTrue())
											Expect(latestCopyStatus).To(Equal(volsyncv1alpha1.LatestCopyStatusValueCompleted))

											latestCopyTrigger, ok := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
											Expect(ok).To(BeTrue())
											Expect(latestCopyTrigger).To(Equal("second-t"))
										})
										It("Should update the latest-copy-trigger and status annotations", func() {
											// See checks in JustBeforeEach
											latestCopyTrigger := sPVC.Annotations[volsyncv1alpha1.LatestCopyTriggerAnnotation]
											userCopyTrigger := sPVC.Annotations[volsyncv1alpha1.CopyTriggerAnnotation]
											Expect(latestCopyTrigger).To(Equal(userCopyTrigger))
										})
									})
								})
							})
						})
					})
				})
			})
		})

		Context("mover Job is handled properly", func() {
			var jobName string
			var cache *corev1.PersistentVolumeClaim
			var sa *corev1.ServiceAccount
			var repo *corev1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-src-" + rs.Name
				cache = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thecache",
						Namespace: ns.Name,
					},
				}
				sPVC.Spec.DeepCopyInto(&cache.Spec)
				sa = &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				repo = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysecret",
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have only the backup action", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("backup"))
				})
				It("should use the specified container image", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Image
					Expect(args).To(Equal(defaultResticContainerImage))
				})
				It("should use the specified service account", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal(sa.Name))
				})
				It("should support pausing", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(1)))

					mover.paused = true
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(0)))

					mover.paused = false
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(1)))
				})

				It("Should have correct volumes", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					volumes := job.Spec.Template.Spec.Volumes
					Expect(len(volumes)).To(Equal(3))
					foundDataVolume := false
					foundCacheVolume := false
					for _, vol := range volumes {
						switch vol.Name {
						case dataVolumeName:
							foundDataVolume = true
							Expect(vol.VolumeSource.PersistentVolumeClaim).ToNot(BeNil())
							Expect(vol.VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(sPVC.GetName()))
							Expect(vol.VolumeSource.PersistentVolumeClaim.ReadOnly).To(Equal(false))
						case resticCache:
							foundCacheVolume = true
							Expect(vol.VolumeSource.PersistentVolumeClaim).ToNot(BeNil())
							Expect(vol.VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(cache.GetName()))
						}
					}
					Expect(foundDataVolume).To(BeTrue())
					Expect(foundCacheVolume).To(BeTrue())
				})

				It("Should not have a PodSecurityContext by default", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					psc := job.Spec.Template.Spec.SecurityContext
					Expect(psc).NotTo(BeNil())
					Expect(psc.RunAsUser).To(BeNil())
					Expect(psc.FSGroup).To(BeNil())
				})

				When("The ReplicationSource CR name is very long", func() {
					BeforeEach(func() {
						rs.Name = "very-long-name-will-cause-job-name-to-be-evenlongerthan63chars"
					})

					It("The job name should be shortened appropriately (should handle long CR names)", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed

						jobs := &batchv1.JobList{}
						Expect(k8sClient.List(ctx, jobs, client.InNamespace(rs.Namespace))).To(Succeed())
						Expect(len(jobs.Items)).To(Equal(1))
						moverJob := jobs.Items[0]

						// Reload the replicationsource to see that it got updated
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())

						Expect(moverJob.GetName()).To(ContainSubstring(utils.GetHashedName(rs.GetName())))
						// Make sure our shortened name is actually short enough
						Expect(len(moverJob.GetName()) > 63).To(BeFalse())
					})
				})

				When("A moverSecurityContext is provided", func() {
					BeforeEach(func() {
						rs.Spec.Restic.MoverSecurityContext = &corev1.PodSecurityContext{
							RunAsUser: ptr.To[int64](7),
							FSGroup:   ptr.To[int64](8),
						}
					})
					It("Should appear in the mover Job", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						psc := job.Spec.Template.Spec.SecurityContext
						Expect(psc).NotTo(BeNil())
						Expect(psc.RunAsUser).NotTo(BeNil())
						Expect(*psc.RunAsUser).To(Equal(int64(7)))
						Expect(psc.FSGroup).NotTo(BeNil())
						Expect(*psc.FSGroup).To(Equal(int64(8)))
					})
				})

				It("Should not have container resourceRequirements set by default", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					Expect(len(job.Spec.Template.Spec.Containers)).To(Equal(1))
					// ResourceRequirements should be the empty/default value
					Expect(job.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{}))
				})
				When("moverResources (resource requirements) are provided", func() {
					BeforeEach(func() {
						rs.Spec.Restic.MoverResources = &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						}
					})
					It("Should use them in the mover job container", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						Expect(len(job.Spec.Template.Spec.Containers)).To(Equal(1))
						// ResourceRequirements should be set
						resourceReqs := job.Spec.Template.Spec.Containers[0].Resources
						Expect(resourceReqs.Limits).To(BeNil()) // No limits were set
						Expect(resourceReqs.Requests).NotTo(BeNil())

						cpuRequest := resourceReqs.Requests[corev1.ResourceCPU]
						Expect(cpuRequest).NotTo(BeNil())
						Expect(cpuRequest).To(Equal(resource.MustParse("50m")))
						memRequest := resourceReqs.Requests[corev1.ResourceMemory]
						Expect(memRequest).NotTo(BeNil())
						Expect(memRequest).To(Equal(resource.MustParse("128Mi")))
					})
				})

				When("The NS allows privileged movers", func() { // Already the case in this block
					It("Should start a privileged mover", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						sc := job.Spec.Template.Spec.Containers[0].SecurityContext
						Expect(sc).NotTo(BeNil())
						Expect(len(sc.Capabilities.Add)).To(BeNumerically(">", 0))
						Expect(sc.RunAsUser).NotTo(BeNil())
						Expect(*sc.RunAsUser).To(Equal(int64(0)))
					})
				})
			})

			When("The source PVC is ROX", func() {
				var roxPVC *corev1.PersistentVolumeClaim
				BeforeEach(func() {
					// Create a ROX PVC to use as the src
					roxScName := "test-rox-storageclass"
					roxPVC = &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "test-rox-pvc-",
							Namespace:    ns.Name,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: &roxScName,
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadOnlyMany,
							},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									"storage": resource.MustParse("1Gi"),
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, roxPVC)).To(Succeed())
				})
				It("Mover job should mount the PVC as read-only", func() {
					j, e := mover.ensureJob(ctx, cache, roxPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					foundDataVolume := false
					for _, vol := range job.Spec.Template.Spec.Volumes {
						if vol.Name == dataVolumeName {
							foundDataVolume = true
							Expect(vol.VolumeSource.PersistentVolumeClaim).ToNot(BeNil())
							Expect(vol.VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(roxPVC.GetName()))
							// The volumes should be mounted read-only
							Expect(vol.VolumeSource.PersistentVolumeClaim.ReadOnly).To(Equal(true))
						}
					}
					Expect(foundDataVolume).To(Equal(true))
				})
			})

			// nolint:dupl
			Context("Unlock tests", func() {
				When("Unlock is used (spec.restic.unlock", func() {
					JustBeforeEach(func() {
						mover.unlock = "unlock-1"
					})
					It("should run a backup with unlock", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
						Expect(mover.shouldUnlock()).To(BeTrue())
						Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
						args := job.Spec.Template.Spec.Containers[0].Args
						Expect(args).To(ConsistOf([]string{"unlock", "backup"}))
						// Mark completed
						job.Status.Succeeded = int32(1)
						Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).NotTo(BeNil())
						Expect(mover.sourceStatus.LastUnlocked).To(Equal("unlock-1"))
					})
				})

				When("another sync is run without changing unlock in the spec", func() {
					JustBeforeEach(func() {
						// Simulating that unlock-2 has already run (i.e. it's in the status)
						mover.unlock = "unlock-2"
						mover.sourceStatus.LastUnlocked = mover.unlock
					})

					It("should run a backup without running unlock", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
						Expect(mover.shouldUnlock()).To(BeFalse())
						Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
						args := job.Spec.Template.Spec.Containers[0].Args
						Expect(args).To(ConsistOf("backup"))
						// Mark completed
						job.Status.Succeeded = int32(1)
						Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).NotTo(BeNil())
						// LastUnlocked should still be the previous value
						Expect(mover.sourceStatus.LastUnlocked).To(Equal("unlock-2"))
					})
				})

				When("another sync is run with a new value of unlock in the spec", func() {
					JustBeforeEach(func() {
						mover.unlock = "unlock-3" // User has requested new unlock

						// Simulating that unlock-2 has already run (i.e. it's in the status)
						mover.sourceStatus.LastUnlocked = "unlock-2"
					})

					It("should run a backup with unlock", func() {
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
						Expect(mover.shouldUnlock()).To(BeTrue())
						Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
						args := job.Spec.Template.Spec.Containers[0].Args
						Expect(args).To(ConsistOf([]string{"unlock", "backup"}))
						// Mark completed
						job.Status.Succeeded = int32(1)
						Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).NotTo(BeNil())
						// LastUnlocked should be updated with the new value
						Expect(mover.sourceStatus.LastUnlocked).To(Equal("unlock-3"))
					})
				})

				When("another sync is run and unlock is cleared from the spec", func() {
					JustBeforeEach(func() {
						mover.unlock = "" // User deleted it from the spec

						// Simulating that unlock-1 has previously run
						mover.sourceStatus.LastUnlocked = "unlock-1"
					})
					It("should run a backup without running unlock", func() {
						mover.unlock = ""

						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
						Expect(mover.shouldUnlock()).To(BeFalse())
						Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
						args := job.Spec.Template.Spec.Containers[0].Args

						Expect(args).To(ConsistOf("backup")) // No unlock
						// Mark completed
						job.Status.Succeeded = int32(1)
						Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).NotTo(BeNil())

						// LastUnlocked should be empty now
						Expect(mover.sourceStatus.LastUnlocked).To(Equal(""))
					})
				})
			})

			When("it's time to prune", func() {
				var lastMonth metav1.Time
				JustBeforeEach(func() {
					lastMonth.Time = time.Now().Add(-28 * 24 * time.Hour)
					// Mover has already been built, so we can't just update
					// rs.Status.Restic.LastPruned
					mover.sourceStatus = &volsyncv1alpha1.ReplicationSourceResticStatus{
						LastPruned: &lastMonth,
					}
				})
				It("should have the backup and prune actions", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(mover.shouldPrune(time.Now())).To(BeTrue())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("backup", "prune"))
					// Mark completed
					job.Status.Succeeded = int32(1)
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).NotTo(BeNil())
					Expect(mover.sourceStatus.LastPruned.After(lastMonth.Time))
				})
			})

			When("Doing a sync when the job already exists", func() {
				JustBeforeEach(func() {
					mover.containerImage = "my-restic-mover-image"

					// Initial job creation
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed

					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(mover.containerImage))
				})

				It("Should recreate the job if job.spec.template needs modification", func() {
					myUpdatedImage := "somenew-restic-mover:latest"

					// change to simulate mover image being updated
					mover.containerImage = myUpdatedImage

					// Mover should get immutable err for updating the image and then delete the job
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).To(HaveOccurred())
					Expect(j).To(BeNil())

					// Make sure job has been deleted
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, nsn, job))).To(BeTrue())

					// Run ensureJob again as the reconciler would do - should recreate the job
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // job hasn't completed

					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(myUpdatedImage))
				})
			})

			When("the job has failed", func() {
				It("should be restarted", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					job.Status.Failed = *job.Spec.BackoffLimit
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

					// 1st reconcile should delete the job
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil())
					// Job should be deleted
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, nsn, job))).To(BeTrue())

					// 2nd reconcile should recreate the job
					j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(job.Status.Failed).To(Equal(int32(0)))
				})
			})
		})
	})

	When("used as source", func() {
		JustBeforeEach(func() {
			// Controller sets status to non-nil
			rs.Status = &volsyncv1alpha1.ReplicationSourceStatus{}
			// Instantiate a restic mover for the tests
			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				false /* UNprivileged */)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("mover Job is handled properly", func() {
			var jobName string
			var cache *corev1.PersistentVolumeClaim
			var sa *corev1.ServiceAccount
			var repo *corev1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-src-" + rs.Name
				cache = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thecache",
						Namespace: ns.Name,
					},
				}
				sPVC.Spec.DeepCopyInto(&cache.Spec)
				sa = &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				repo = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysecret",
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
			})

			It("Should run unprivileged by default", func() {
				j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo, nil)
				Expect(e).NotTo(HaveOccurred())
				Expect(j).To(BeNil()) // hasn't completed
				nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
				job = &batchv1.Job{}
				Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

				sc := job.Spec.Template.Spec.Containers[0].SecurityContext
				Expect(sc).NotTo(BeNil())
				Expect(len(sc.Capabilities.Add)).To(Equal(0))
				Expect(sc.RunAsUser).To(BeNil())
			})
		})
	})
})

var _ = Describe("Restic as a destination", func() {
	var ctx = context.TODO()
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rd *volsyncv1alpha1.ReplicationDestination
	var mover *Mover
	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())

		// Scaffold ReplicationDestination
		rd = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rd",
				Namespace: ns.Name,
			},
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				Trigger: &volsyncv1alpha1.ReplicationDestinationTriggerSpec{},
				Restic:  &volsyncv1alpha1.ReplicationDestinationResticSpec{},
			},
		}
	})
	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, rd)).To(Succeed())
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})
	When("used as destination", func() {
		JustBeforeEach(func() {
			// Controller sets status to non-nil
			rd.Status = &volsyncv1alpha1.ReplicationDestinationStatus{}
			// Instantiate a restic mover for the tests
			m, err := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("Restic cache is created correctly", func() {
			var dataPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				am := corev1.ReadWriteMany
				rd.Spec.Restic.AccessModes = []corev1.PersistentVolumeAccessMode{
					am,
				}
				destVolCap := resource.MustParse("6Gi")
				rd.Spec.Restic.Capacity = &destVolCap
			})

			JustBeforeEach(func() {
				var err error
				dataPVC, err = mover.ensureDestinationPVC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should create old cache pvc", func() {
				cache, err := mover.ensureCache(ctx, dataPVC, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(cache.GetName()).To(ContainSubstring("-cache"))
				Expect(cache.GetName()).To(ContainSubstring("-dst-"))
			})
		})

		When("no destination volume is supplied", func() {
			var destVolCap resource.Quantity
			var am corev1.PersistentVolumeAccessMode
			BeforeEach(func() {
				am = corev1.ReadWriteMany
				rd.Spec.Restic.AccessModes = []corev1.PersistentVolumeAccessMode{
					am,
				}
				destVolCap = resource.MustParse("6Gi")
				rd.Spec.Restic.Capacity = &destVolCap
			})
			It("creates a dynamic PVC", func() {
				pvc, e := mover.ensureDestinationPVC(ctx)
				Expect(e).NotTo(HaveOccurred())
				Expect(pvc).NotTo(BeNil())
				Expect(pvc.Spec.AccessModes).To(ConsistOf(am))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(destVolCap))

				// Cleanup label should not be on this PVC
				Expect(pvc.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
			})

			When("cleanupTempPVC is set to true", func() {
				BeforeEach(func() {
					rd.Spec.Restic.CleanupTempPVC = true
				})
				It("The dynamic PVC should be marked for deletion", func() {
					pvc, e := mover.ensureDestinationPVC(ctx)
					Expect(e).NotTo(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					// Cleanup label should be set on this PVC
					Expect(pvc.Labels).To(HaveKey("volsync.backube/cleanup"))
				})
			})
		})
		When("a destination volume is supplied", func() {
			var dPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				dPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dest",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				rd.Spec.Restic.DestinationPVC = &dPVC.Name
			})
			It("is used directly", func() {
				pvc, e := mover.ensureDestinationPVC(ctx)
				Expect(e).NotTo(HaveOccurred())
				Expect(pvc).NotTo(BeNil())
				Expect(pvc.Name).To(Equal(dPVC.Name))
			})

			// We will NOT cleanup a users destination PVC, only ones we create dynamically
			// So we should ignore the cleanupTempPVC setting if destinationPVC is set
			When("cleanupTempPVC is set to true", func() {
				BeforeEach(func() {
					rd.Spec.Restic.CleanupTempPVC = true
				})
				It("The user supplied PVC should NOT be marked for deletion", func() {
					pvc, e := mover.ensureDestinationPVC(ctx)
					Expect(e).NotTo(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					// Cleanup label should NOT be set on this PVC
					Expect(pvc.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
		})
		When("the service account is created", func() {
			It("exists", func() {
				sa, err := mover.saHandler.Reconcile(ctx, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(sa).NotTo(BeNil())
				saName := sa.Name
				sa2 := &corev1.ServiceAccount{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      saName,
					Namespace: ns.Name,
				}, sa2)).To(Succeed())
				Expect(sa2.Name).To(Equal(sa.Name))
			})
		})
		When("A user supplied moverServiceAccount is set in the spec", func() {
			userSuppliedMoverSvcAccount := "cust-svc-acct"
			BeforeEach(func() {
				// Update rsSpec to set our own svc account
				rd.Spec.Restic.MoverServiceAccount = &userSuppliedMoverSvcAccount
			})

			When("The mover service account does not exist", func() {
				It("The saHandler should fail to reconcile", func() {
					sa, err := mover.saHandler.Reconcile(ctx, logger)
					Expect(sa).To(BeNil())
					Expect(err).To(HaveOccurred())
					Expect(err).To(HaveOccurred())
				})
			})

			When("The mover service account exists", func() {
				BeforeEach(func() {
					// Create the svc account
					userSvcAccount := &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userSuppliedMoverSvcAccount,
							Namespace: ns.Name,
						},
					}
					Expect(k8sClient.Create(ctx, userSvcAccount)).To(Succeed())
				})
				It("Should use the supplied service account", func() {
					sa, err := mover.saHandler.Reconcile(ctx, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(sa.GetName()).To(Equal(userSuppliedMoverSvcAccount))
				})
			})
		})

		Context("mover Job is handled properly", func() {
			var jobName string
			var dPVC *corev1.PersistentVolumeClaim
			var cache *corev1.PersistentVolumeClaim
			var sa *corev1.ServiceAccount
			var repo *corev1.Secret
			var job *batchv1.Job
			var caSecret *corev1.Secret
			var caConfigMap *corev1.ConfigMap
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-dst-" + rd.Name
				dPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dest",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				cache = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thecache",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				sa = &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				repo = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysecret",
						Namespace: ns.Name,
					},
				}
				caSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "theca",
						Namespace: ns.Name,
					},
					StringData: map[string]string{
						"key": "value",
					},
				}
				caConfigMap = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cmca",
						Namespace: ns.Name,
					},
					Data: map[string]string{
						"key": "myvalue",
					},
				}
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
				Expect(k8sClient.Create(ctx, caSecret)).To(Succeed())
				Expect(k8sClient.Create(ctx, caConfigMap)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have only the restore action", func() {
					j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("restore"))
				})
			})
			When("a custom CA is not supplied", func() {
				It("Should not attempt to update the podspec in the mover job", func() {
					var customCA volsyncv1alpha1.CustomCASpec // No CustomCA, not initializing w any values
					customCAObj, err := utils.ValidateCustomCA(ctx, k8sClient, logger, ns.Name, customCA)
					Expect(err).NotTo(HaveOccurred())
					Expect(customCAObj).To(BeNil())
				})
			})
			When("a custom CA is supplied", func() {
				var customCASpec volsyncv1alpha1.CustomCASpec
				JustBeforeEach(func() {
					mover.customCASpec = customCASpec
					customCaObj, err := utils.ValidateCustomCA(ctx, k8sClient, logger, ns.Name, mover.customCASpec)
					Expect(err).NotTo(HaveOccurred())

					// Common checks for customCA (configCA as secret or configmap)
					j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, customCaObj)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Location in Env variable
					Expect(job.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
						Name:  "CUSTOM_CA",
						Value: path.Join(resticCAMountPath, resticCAFilename),
					}))
				})

				When("a custom CA is supplied as a secret", func() {
					BeforeEach(func() {
						customCASpec = volsyncv1alpha1.CustomCASpec{SecretName: caSecret.Name, Key: "key"}
					})
					It("should be mounted in the container", func() {
						// See common checks in JustBeforeEach() above

						// Check that Secret added to Pod as volume
						var volName string
						for _, v := range job.Spec.Template.Spec.Volumes {
							if v.Secret != nil && v.Secret.SecretName == caSecret.Name {
								volName = v.Name
							}
						}
						Expect(volName).NotTo(BeEmpty())

						// Check that secret volume is mounted to container
						var mountPath string
						for _, v := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
							if v.Name == volName {
								mountPath = v.MountPath
							}
						}
						Expect(mountPath).To(Equal(resticCAMountPath))
					})
				})
				When("a custom CA is supplied as a ConfigMap", func() {
					BeforeEach(func() {
						customCASpec = volsyncv1alpha1.CustomCASpec{ConfigMapName: caConfigMap.Name, Key: "key"}
					})
					It("should be mounted in the container", func() {
						// See common checks in JustBeforeEach() above

						// Check that ConfigMap added to Pod as volume
						var volName string
						for _, v := range job.Spec.Template.Spec.Volumes {
							if v.ConfigMap != nil && v.ConfigMap.Name == caConfigMap.Name {
								volName = v.Name
							}
						}
						Expect(volName).NotTo(BeEmpty())
						// Mounted to container
						var mountPath string
						for _, v := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
							if v.Name == volName {
								mountPath = v.MountPath
							}
						}
						Expect(mountPath).To(Equal(resticCAMountPath))
					})
				})
			})
			When("RCLONE_ env vars are in the restic secret", func() {
				BeforeEach(func() {
					repo.StringData = map[string]string{
						"RESTIC_REPOSITORY":                "myreponame",
						"RESTIC_PASSWORD":                  "abc123",
						"RCLONE_CONFIG_NAS_MD5SUM_COMMAND": "md5",
						"RCLONE_CONFIG_NAS_PORT":           "9999",
						"RCLONE_CONFIG_NAS_PASS":           "naspass",
					}
				})

				It("Should set the env vars in the mover job pod", func() {
					j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					env := job.Spec.Template.Spec.Containers[0].Env

					// RESTIC_REPOSITORY and _PASSWORD are mandatory
					verifyEnvVarFromSecret(env, "RESTIC_REPOSITORY", repo.GetName(), false)
					verifyEnvVarFromSecret(env, "RESTIC_PASSWORD", repo.GetName(), false)

					// RCLONE env vars should be optional
					verifyEnvVarFromSecret(env, "RCLONE_CONFIG_NAS_MD5SUM_COMMAND", repo.GetName(), true)
					verifyEnvVarFromSecret(env, "RCLONE_CONFIG_NAS_PORT", repo.GetName(), true)
					verifyEnvVarFromSecret(env, "RCLONE_CONFIG_NAS_PASS", repo.GetName(), true)
				})
			})
			Context("Handling GCS credentials", func() {
				When("no credentials are provided", func() {
					It("shouldn't mount the Secret", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						for _, env := range job.Spec.Template.Spec.Containers[0].Env {
							Expect(env.Name).NotTo(Equal("GOOGLE_APPLICATION_CREDENTIALS"))
						}
						for _, v := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
							Expect(v.Name).NotTo(Equal("gcs-credentials"))
						}
						for _, v := range job.Spec.Template.Spec.Volumes {
							Expect(v.Name).NotTo(Equal("gcs-credentials"))
						}
					})
				})
				When("credentials are provided", func() {
					BeforeEach(func() {
						repo.Data = map[string][]byte{
							"GOOGLE_APPLICATION_CREDENTIALS": []byte("dummy"),
						}
					})
					It("should mount the Secret", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						found := false
						for _, env := range job.Spec.Template.Spec.Containers[0].Env {
							if env.Name == "GOOGLE_APPLICATION_CREDENTIALS" {
								found = true
								Expect(env.Value).To(Equal(path.Join(credentialDir, gcsCredentialFile)))
							}
						}
						Expect(found).To(BeTrue())
						found = false
						for _, v := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
							if v.Name == "gcs-credentials" {
								found = true
								Expect(v.MountPath).To(Equal(credentialDir))
							}
						}
						Expect(found).To(BeTrue())
						found = false
						for _, v := range job.Spec.Template.Spec.Volumes {
							if v.Name == "gcs-credentials" {
								found = true
								Expect(v.Secret).NotTo(BeNil())
								Expect(v.Secret.Items).To(ContainElement(corev1.KeyToPath{
									Key:  "GOOGLE_APPLICATION_CREDENTIALS",
									Path: gcsCredentialFile,
								}))
							}
						}
						Expect(found).To(BeTrue())
					})
				})
			})

			Context("Restore options", func() {
				When("No restore options are specified", func() {
					It("should set env vars related to restore options with defaults", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						var forgetOptions *corev1.EnvVar
						var restoreAsOf *corev1.EnvVar
						var selectPrevious *corev1.EnvVar
						var restoreOptions *corev1.EnvVar

						envVars := job.Spec.Template.Spec.Containers[0].Env
						for i := range envVars {
							envVar := envVars[i]
							switch envVar.Name {
							case "FORGET_OPTIONS":
								forgetOptions = &envVar
							case "RESTORE_AS_OF":
								restoreAsOf = &envVar
							case "SELECT_PREVIOUS":
								selectPrevious = &envVar
							case "RESTORE_OPTIONS":
								restoreOptions = &envVar
							}
						}

						Expect(forgetOptions).NotTo(BeNil())
						Expect(forgetOptions.Value).To(Equal("--keep-last 1")) // Default value

						Expect(restoreAsOf).NotTo(BeNil())
						Expect(restoreAsOf.Value).To(Equal("")) // Default value

						Expect(selectPrevious).NotTo(BeNil())
						Expect(selectPrevious.Value).To(Equal("0")) // Default value

						Expect(restoreOptions).NotTo(BeNil())
						Expect(restoreOptions.Value).To(Equal("")) // Default value
					})
				})
				When("Restore option of enableFileDeletion is specified", func() {
					BeforeEach(func() {
						rd.Spec.Restic.EnableFileDeletion = true
					})
					It("should set RESTORE_OPTIONS env var with delete flag", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						var restoreOptions *corev1.EnvVar
						envVars := job.Spec.Template.Spec.Containers[0].Env
						for i := range envVars {
							envVar := envVars[i]
							if envVar.Name == "RESTORE_OPTIONS" {
								restoreOptions = &envVar
							}
						}
						Expect(restoreOptions).NotTo(BeNil())
						Expect(restoreOptions.Value).To(Equal("--delete"))
					})
				})
			})

			Context("Cluster wide proxy settings", func() {
				When("no proxy env vars are set on the volsync controller", func() {
					It("shouldn't set any proxy env vars on the mover job", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						// No proxy env vars should be set by default
						envVars := job.Spec.Template.Spec.Containers[0].Env
						for _, envVar := range envVars {
							Expect(strings.ToLower(envVar.Name)).NotTo(ContainSubstring("proxy"))
						}
					})
				})

				When("proxy env vars are set on the volsync controller", func() {
					httpProxy := "http://myproxy:1234"
					httpsProxy := "https://10.10.10.1"
					noProxy := "*.abc.com, 10.11.11.200"
					BeforeEach(func() {
						os.Setenv("HTTP_PROXY", httpProxy)
						os.Setenv("HTTPS_PROXY", httpsProxy)
						os.Setenv("NO_PROXY", noProxy)
					})
					AfterEach(func() {
						os.Unsetenv("HTTP_PROXY")
						os.Unsetenv("HTTPS_PROXY")
						os.Unsetenv("NO_PROXY")
					})

					It("should set the corresponding proxy env vars on the mover job", func() {
						j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo, nil)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						// No proxy env vars should be set by default
						envVars := job.Spec.Template.Spec.Containers[0].Env
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "HTTPS_PROXY", Value: httpsProxy}))
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "https_proxy", Value: httpsProxy}))
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "HTTP_PROXY", Value: httpProxy}))
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "http_proxy", Value: httpProxy}))
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "NO_PROXY", Value: noProxy}))
						Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "no_proxy", Value: noProxy}))
					})
				})
			})
		})
	})
})

func verifyEnvVarFromSecret(env []corev1.EnvVar, envVarName string, secretName string, optional bool) {
	Expect(env).To(ContainElement(corev1.EnvVar{
		Name: envVarName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      envVarName,
				Optional: &optional,
			},
		},
	}))
}
