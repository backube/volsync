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

package rclone

import (
	"flag"
	"os"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
)

const (
	timeout  = "30s"
	interval = "1s"
)

var testRcloneConfig = "testrclonesecret"
var testRcloneConfigSection = "testrcloneconfigsection"
var testRcloneDestPath = "/test/destpath"
var emptyString = ""

var _ = Describe("Rclone properly registers", func() {
	When("Rclone's registration function is called", func() {
		BeforeEach(func() {
			Expect(Register()).To(Succeed())
		})

		It("is added to the mover catalog", func() {
			found := false
			for _, v := range mover.Catalog {
				if _, ok := v.(*Builder); ok {
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})

var _ = Describe("Rclone init flags and env vars", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("Rclone builder inits flags", func() {
		var builderForInitTests *Builder
		var testPflagSet *pflag.FlagSet
		BeforeEach(func() {
			os.Unsetenv(rcloneContainerImageEnvVar)

			// Instantiate new viper instance and flagset instance just for this test
			testViper := viper.New()
			testFlagSet := flag.NewFlagSet("testflagsetrclone", flag.ExitOnError)

			// New Builder for this test - use testViper and testFlagSet so we can modify
			// flags for these tests without modifying global flags and potentially affecting other tests
			var err error
			builderForInitTests, err = newBuilder(testViper, testFlagSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(builderForInitTests).NotTo(BeNil())

			// code here (see main.go) for viper to bind cmd line flags (including those
			// defined in the mover Register() func)
			// Bind viper to a new set of flags so each of these tests can get their own
			testPflagSet = pflag.NewFlagSet("testpflagsetrclone", pflag.ExitOnError)
			testPflagSet.AddGoFlagSet(testFlagSet)
			Expect(testViper.BindPFlags(testPflagSet)).To(Succeed())
		})

		AfterEach(func() {
			os.Unsetenv(rcloneContainerImageEnvVar)
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
					Rclone: &volsyncv1alpha1.ReplicationSourceRcloneSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationSourceStatus{}, // Controller sets status to non-nil
			}
			sourceMover, err := builderForInitTests.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs)
			Expect(err).NotTo(HaveOccurred())
			Expect(sourceMover).NotTo(BeNil())
			sourceRcloneMover, _ := sourceMover.(*Mover)
			Expect(sourceRcloneMover.containerImage).To(Equal(builderForInitTests.getRcloneContainerImage()))

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: "testing",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Trigger: &volsyncv1alpha1.ReplicationDestinationTriggerSpec{},
					Rclone:  &volsyncv1alpha1.ReplicationDestinationRcloneSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationDestinationStatus{}, // Controller sets status to non-nil
			}
			destMover, err := builderForInitTests.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd)
			Expect(err).NotTo(HaveOccurred())
			Expect(destMover).NotTo(BeNil())
			destRcloneMover, _ := destMover.(*Mover)
			Expect(destRcloneMover.containerImage).To(Equal(builderForInitTests.getRcloneContainerImage()))
		})

		Context("When no command line flag or ENV var is specified", func() {
			It("Should use the default rclone container image", func() {
				Expect(builderForInitTests.getRcloneContainerImage()).To(Equal(defaultRcloneContainerImage))
			})
		})

		Context("When rclone container image command line flag is specified", func() {
			const cmdLineOverrideImageName = "test-rclone-image-name:cmdlineoverride"
			BeforeEach(func() {
				// Manually set the value of the command line flag
				Expect(testPflagSet.Set("rclone-container-image", cmdLineOverrideImageName)).To(Succeed())
			})
			It("Should use the rclone container image set by the cmd line flag", func() {
				Expect(builderForInitTests.getRcloneContainerImage()).To(Equal(cmdLineOverrideImageName))
			})

			Context("And env var is set", func() {
				const envVarOverrideShouldBeIgnored = "test-rclone-image-name:donotuseme"
				BeforeEach(func() {
					os.Setenv(rcloneContainerImageEnvVar, envVarOverrideShouldBeIgnored)
				})
				It("Should still use the cmd line flag instead of the env var", func() {
					Expect(builderForInitTests.getRcloneContainerImage()).To(Equal(cmdLineOverrideImageName))
				})
			})
		})

		Context("When rclone container image cmd line flag is not set and env var is", func() {
			const envVarOverrideImageName = "test-rclone-image-name:setbyenvvar"
			BeforeEach(func() {
				os.Setenv(rcloneContainerImageEnvVar, envVarOverrideImageName)
			})
			It("Should use the value from the env var", func() {
				Expect(builderForInitTests.getRcloneContainerImage()).To(Equal(envVarOverrideImageName))
			})
		})
	})
})

var _ = Describe("Rclone ignores other movers", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("An RS isn't for rclone", func() {
		It("is ignored", func() {
			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr",
					Namespace: "blah",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Rclone: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
	When("An RD isn't for rclone", func() {
		It("is ignored", func() {
			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x",
					Namespace: "y",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rclone: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Rclone as a source", func() {
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
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": resource.MustParse("7Gi"),
					},
				},
				StorageClassName: &sc,
			},
		}
		Expect(k8sClient.Create(ctx, sPVC)).To(Succeed())
		Eventually(func() error {
			pvc := &corev1.PersistentVolumeClaim{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sPVC), pvc)
			return err
		}, timeout, interval).Should(Succeed())

		// Scaffold ReplicationSource
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rs",
				Namespace: ns.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: sPVC.Name,
				Trigger:   &volsyncv1alpha1.ReplicationSourceTriggerSpec{},
				Rclone:    &volsyncv1alpha1.ReplicationSourceRcloneSpec{},
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
			// Instantiate a rclone mover for the tests
			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("validate rclone spec", func() {
			When("no rcloneConfig (secret) is specified", func() {
				BeforeEach(func() {
					// Not setting rs.Spec.RClone.RcloneConfig
					rs.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
					rs.Spec.Rclone.RcloneDestPath = &testRcloneDestPath
				})
				It("validation should fail", func() {
					err := mover.validateSpec()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Rclone config secret name"))
				})
			})
			When("empty rcloneConfig (secret) is specified", func() {
				BeforeEach(func() {
					// Setting rs.Spec.RClone.RcloneConfig to empty string
					rs.Spec.Rclone.RcloneConfig = &emptyString
					rs.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
					rs.Spec.Rclone.RcloneDestPath = &testRcloneDestPath
				})
				It("validation should fail", func() {
					err := mover.validateSpec()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Rclone config secret name"))
				})
			})
			When("no rcloneConfigSection is specified", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.RcloneConfig = &testRcloneConfig
					// No rcloneconfigsection
					rs.Spec.Rclone.RcloneDestPath = &testRcloneDestPath
				})
				It("validation should fail", func() {
					err := mover.validateSpec()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Rclone config section name"))
				})
			})
			When("no rcloneDestPath is specified", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.RcloneConfig = &testRcloneConfig
					rs.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
					// No rcloneDestPath
				})
				It("validation should fail", func() {
					err := mover.validateSpec()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Rclone destination"))
				})
			})
		})
		Context("validate rclone config secret", func() {
			var rcloneConfigSecret *corev1.Secret
			BeforeEach(func() {
				rcloneConfigSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testRcloneConfig,
						Namespace: ns.Name,
					},
				}
				Expect(k8sClient.Create(ctx, rcloneConfigSecret)).To(Succeed())
				rs.Spec.Rclone.RcloneConfig = &rcloneConfigSecret.Name
				rs.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
				rs.Spec.Rclone.RcloneDestPath = &testRcloneDestPath
			})

			When("rclone config secret does not exist", func() {
				BeforeEach(func() {
					badSecretName := "thisdoesnotexist"
					rs.Spec.Rclone.RcloneConfig = &badSecretName
				})
				It("validateRcloneConfig should fail", func() {
					_, err := mover.validateRcloneConfig(ctx)
					Expect(err).To(HaveOccurred())
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})
			})

			When("rclone config secret exists", func() {
				It("Should fail validation if rclone.conf field not defined in the secret", func() {
					secret, err := mover.validateRcloneConfig(ctx)
					Expect(err).To(HaveOccurred())
					Expect(secret).To(BeNil())
					Expect(err.Error()).To(ContainSubstring("secret should have fields"))
					Expect(err.Error()).To(ContainSubstring("rclone.conf"))
				})

				It("Should pass validation when rclone.confg is defined in the secret", func() {
					rcloneConfigSecret.StringData = map[string]string{
						"rclone.conf": "fakeconfig stuff here",
					}
					Expect(k8sClient.Update(ctx, rcloneConfigSecret)).To(Succeed())
					Eventually(func() bool {
						secret, err := mover.validateRcloneConfig(ctx)
						return secret != nil && err == nil
					}, "5s", "1s").Should(BeTrue())
				})
			})
		})
		Context("Source volume is handled properly", func() {
			When("CopyMethod is None", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.CopyMethod = volsyncv1alpha1.CopyMethodNone
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
					rs.Spec.Rclone.CopyMethod = volsyncv1alpha1.CopyMethodDirect
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
					rs.Spec.Rclone.CopyMethod = volsyncv1alpha1.CopyMethodClone
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
			})
			When("CopyMethod is Snapshot", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
				})
				It("the source is not used as data pvc, snapshot is created", func() {
					_, err := mover.ensureSourcePVC(ctx)
					Expect(err).ToNot(HaveOccurred())

					// Set snapshot to be bound so the ensureSourcePVC can proceed
					snapshots := &snapv1.VolumeSnapshotList{}
					Eventually(func() []snapv1.VolumeSnapshot {
						_ = k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))
						return snapshots.Items
					}, timeout, interval).Should(Not(BeEmpty()))
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
			})
		})

		Context("ServiceAccount, Role, RoleBinding are handled properly", func() {
			It("Should create a service account", func() {
				var sa *corev1.ServiceAccount
				var err error
				Eventually(func() bool {
					sa, err = mover.ensureSA(ctx)
					return sa != nil && err == nil
				}, timeout, interval).Should(BeTrue())
				Expect(err).ToNot(HaveOccurred())

				validateSaRoleAndRoleBinding(sa, ns.GetName())
			})
		})

		Context("Mover Job is handled properly", func() {
			var jobName string
			var sa *corev1.ServiceAccount
			var rcloneConfigSecret *corev1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				rs.Spec.Rclone.RcloneConfig = &testRcloneConfig
				rs.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
				rs.Spec.Rclone.RcloneDestPath = &testRcloneDestPath

				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-rclone-src-" + rs.Name

				sa = &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				rcloneConfigSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testRcloneConfig,
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, rcloneConfigSecret)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have the command defined properly", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						return k8sClient.Get(ctx, nsn, job)
					}).Should(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(Equal(1))
					Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal(
						[]string{"/bin/bash", "-c", "./active.sh"}))
				})

				It("should use the specified container image", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(defaultRcloneContainerImage))
				})

				It("should use the specified service account", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal(sa.Name))
				})

				It("should have the correct env vars", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())

					// Validate job env vars
					validateJobEnvVars(job.Spec.Template.Spec.Containers[0].Env, true)
				})

				It("Should have correct volume mounts", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())

					c := job.Spec.Template.Spec.Containers[0]
					// Validate job volume mounts
					Expect(len(c.VolumeMounts)).To(Equal(2))
					foundDataVolumeMount := false
					foundRcloneSecretVolumeMount := false
					for _, volMount := range c.VolumeMounts {
						if volMount.Name == dataVolumeName {
							foundDataVolumeMount = true
							Expect(volMount.MountPath).To(Equal(mountPath))
						} else if volMount.Name == rcloneSecret {
							foundRcloneSecretVolumeMount = true
							Expect(volMount.MountPath).To(Equal("/rclone-config/"))
						}
					}
					Expect(foundDataVolumeMount).To(BeTrue())
					Expect(foundRcloneSecretVolumeMount).To(BeTrue())
				})

				It("Should have correct volumes", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())

					volumes := job.Spec.Template.Spec.Volumes
					Expect(len(volumes)).To(Equal(2))
					foundDataVolume := false
					foundRcloneSecretVolume := false
					for _, vol := range volumes {
						if vol.Name == dataVolumeName {
							foundDataVolume = true
							Expect(vol.VolumeSource.PersistentVolumeClaim).ToNot(BeNil())
							Expect(vol.VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(sPVC.GetName()))
						} else if vol.Name == rcloneSecret {
							foundRcloneSecretVolume = true
							Expect(vol.VolumeSource.Secret).ToNot(BeNil())
							Expect(vol.VolumeSource.Secret.SecretName).To(Equal(testRcloneConfig))
						}
					}
					Expect(foundDataVolume).To(BeTrue())
					Expect(foundRcloneSecretVolume).To(BeTrue())
				})

				It("Should have correct labels", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())

					// It should be marked for cleaned up
					Expect(job.Labels).To(HaveKey("volsync.backube/cleanup"))

					Expect(job.Labels).To(HaveKeyWithValue(
						utils.VolsyncCreatedByLabelKey, utils.VolsyncCreatedByLabelValue))

					// Pod template spec should also have the created-by label
					Expect(job.Spec.Template.Labels).To(HaveKeyWithValue(
						utils.VolsyncCreatedByLabelKey, utils.VolsyncCreatedByLabelValue))
				})

				It("should support pausing", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(1)))

					mover.paused = true
					Eventually(func() int32 {
						j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						err := k8sClient.Get(ctx, nsn, job)
						Expect(err).ToNot(HaveOccurred())
						return *job.Spec.Parallelism
					}).Should(Equal(int32(0)))

					mover.paused = false
					Eventually(func() int32 {
						j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						err := k8sClient.Get(ctx, nsn, job)
						Expect(err).ToNot(HaveOccurred())
						return *job.Spec.Parallelism
					}).Should(Equal(int32(1)))
				})
			})

			When("the job has failed", func() {
				It("should be restarted", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						if err := k8sClient.Get(ctx, nsn, job); err != nil {
							return err
						}
						job.Status.Failed = *job.Spec.BackoffLimit
						err := k8sClient.Status().Update(ctx, job)
						return err
					}, timeout, interval).Should(Succeed())
					Eventually(func() int32 {
						j, e := mover.ensureJob(ctx, sPVC, sa, rcloneConfigSecret) // Using sPVC as dataPVC (i.e. direct)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil())
						e = k8sClient.Get(ctx, nsn, job)
						if e != nil {
							return 99
						}
						return job.Status.Failed
					}, timeout, interval).Should(Equal(int32(0)))
				})
			})
		})
	})
})

var _ = Describe("Rclone as a destination", func() {
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
				Rclone:  &volsyncv1alpha1.ReplicationDestinationRcloneSpec{},
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
			m, err := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("Dest volume is handled properly", func() {
			When("no destination volume is supplied", func() {
				var cap resource.Quantity
				var am corev1.PersistentVolumeAccessMode
				BeforeEach(func() {
					am = corev1.ReadWriteMany
					rd.Spec.Rclone.AccessModes = []corev1.PersistentVolumeAccessMode{
						am,
					}
					cap = resource.MustParse("6Gi")
					rd.Spec.Rclone.Capacity = &cap
				})
				It("creates a temporary PVC", func() {
					pvc, e := mover.ensureDestinationPVC(ctx)
					Expect(e).NotTo(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					Expect(pvc.Spec.AccessModes).To(ConsistOf(am))
					Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(cap))
					// It should NOT be marked for cleaned up
					Expect(pvc.Labels).ToNot(HaveKey("volsync.backube/cleanup"))
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
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"storage": resource.MustParse("1Gi"),
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
					rd.Spec.Rclone.DestinationPVC = &dPVC.Name
				})
				It("is used directly", func() {
					var pvc *corev1.PersistentVolumeClaim
					Eventually(func() error {
						tempPVC, e := mover.ensureDestinationPVC(ctx)
						if e == nil {
							pvc = tempPVC
						}
						return e
					}).ShouldNot(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					Expect(pvc.Name).To(Equal(dPVC.Name))
					// It's not owned by the CR
					Expect(pvc.OwnerReferences).To(BeEmpty())
					// It won't be cleaned up at the end of the transfer
					Expect(pvc.Labels).NotTo(HaveKey("volsync.backube/cleanup"))

				})
			})
		})
		Context("ServiceAccount, Role, RoleBinding are handled properly", func() {
			It("Should create a service account", func() {
				var sa *corev1.ServiceAccount
				var err error
				Eventually(func() bool {
					sa, err = mover.ensureSA(ctx)
					return sa != nil && err == nil
				}, timeout, interval).Should(BeTrue())
				Expect(err).ToNot(HaveOccurred())

				validateSaRoleAndRoleBinding(sa, ns.GetName())
			})
		})
		Context("Mover Job is handled properly", func() {
			var jobName string
			var dPVC *corev1.PersistentVolumeClaim
			var sa *corev1.ServiceAccount
			var rcloneConfigSecret *corev1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				rd.Spec.Rclone.RcloneConfig = &testRcloneConfig
				rd.Spec.Rclone.RcloneConfigSection = &testRcloneConfigSection
				rd.Spec.Rclone.RcloneDestPath = &testRcloneDestPath

				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-rclone-dst-" + rd.Name
				dPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dest",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
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
				rcloneConfigSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testRcloneConfig,
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, rcloneConfigSecret)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have the correct env vars", func() {
					j, e := mover.ensureJob(ctx, dPVC, sa, rcloneConfigSecret)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())

					// Validate job env vars
					validateJobEnvVars(job.Spec.Template.Spec.Containers[0].Env, false)
				})
			})
		})

		Context("Cleanup is handled properly", func() {
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
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				rd.Spec.Rclone.DestinationPVC = &dPVC.Name

				Eventually(func() error {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dPVC), dPVC)
					if err != nil {
						return err
					}
					// Update this PVC manually for the test to put the snapshot annotation on it
					if dPVC.Annotations == nil {
						dPVC.Annotations = make(map[string]string)
					}
					dPVC.Annotations["volsync.backube/snapname"] = "testisafakesnapshotannoation"
					return k8sClient.Update(ctx, dPVC)
				}, timeout, interval).Should(Succeed())

			})
			JustBeforeEach(func() {
				uid := rd.GetUID() // UID will only be here after RD is created (hence not putting in BeforeEach)
				// Create some snapshots to test cleanup snap1 will be our fake "oldImage", snap2 will be "latestImage"
				snap1 := &snapv1.VolumeSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mytestsnap1",
						Namespace: ns.Name,
						Labels:    map[string]string{"volsync.backube/ownedby": string(uid)},
					},
					Spec: snapv1.VolumeSnapshotSpec{
						Source: snapv1.VolumeSnapshotSource{
							PersistentVolumeClaimName: pointer.String("dummy"),
						},
					},
				}
				Expect(k8sClient.Create(ctx, snap1)).To(Succeed())
				snap2 := &snapv1.VolumeSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mytestsnap2",
						Namespace: ns.Name,
						Labels:    map[string]string{"volsync.backube/ownedby": string(uid)},
					},
					Spec: snapv1.VolumeSnapshotSpec{
						Source: snapv1.VolumeSnapshotSource{
							PersistentVolumeClaimName: pointer.String("dummy2"),
						},
					},
				}
				Expect(k8sClient.Create(ctx, snap2)).To(Succeed())

				snapshots := &snapv1.VolumeSnapshotList{}
				Eventually(func() int {
					_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
					return len(snapshots.Items)
				}, timeout, interval).Should(Equal(2))

				// Mark prev snapshot (snap1) for cleanup
				oldSnap := &corev1.TypedLocalObjectReference{
					APIGroup: &snapv1.SchemeGroupVersion.Group,
					Kind:     "VolumeSnapshot",
					Name:     snap1.Name,
				}
				latestSnap := &corev1.TypedLocalObjectReference{
					APIGroup: &snapv1.SchemeGroupVersion.Group,
					Kind:     "VolumeSnapshot",
					Name:     snap2.Name,
				}
				Expect(utils.MarkOldSnapshotForCleanup(ctx, k8sClient, logger, rd, oldSnap, latestSnap)).To(Succeed())

				//Reload snap2 and update status
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snap2), snap2)).To(Succeed())
				// Update RD status to indicate snap3 is the latestImage
				rd.Status.LatestImage = &corev1.TypedLocalObjectReference{
					APIGroup: &snapv1.SchemeGroupVersion.Group,
					Kind:     snap2.Kind,
					Name:     snap2.Name,
				}
				Expect(k8sClient.Status().Update(ctx, rd)).To(Succeed())
			})
			It("Should remove snapshot annotations from dest pvc", func() {
				result, err := mover.Cleanup(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Completed).To(BeTrue())
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(dPVC), dPVC)
				}, timeout, interval).Should(Succeed())
				Expect(dPVC.GetAnnotations()["volsync.backube/snap"])
				Expect(dPVC.Annotations).ToNot(HaveKey("volsync.backube/snapname"))
			})
			It("Should remove old snapshot", func() {
				result, err := mover.Cleanup(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Completed).To(BeTrue())

				snapshots := &snapv1.VolumeSnapshotList{}
				Eventually(func() int {
					_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
					return len(snapshots.Items)
				}, timeout, interval).Should(Equal(1))
				// Snapshot left should be our "latestImage"
				Expect(snapshots.Items[0].Name).To(Equal("mytestsnap2"))
			})
		})
	})
})

func validateSaRoleAndRoleBinding(sa *corev1.ServiceAccount, namespace string) {
	Expect(sa).ToNot(BeNil())

	// Ensure SA, role & rolebinding were created
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.GetName(),
			Namespace: namespace,
		},
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.GetName(),
			Namespace: namespace,
		},
	}
	Eventually(func() bool {
		err1 := k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), sa)
		err2 := k8sClient.Get(ctx, client.ObjectKeyFromObject(role), role)
		err3 := k8sClient.Get(ctx, client.ObjectKeyFromObject(roleBinding), roleBinding)
		return err1 == nil && err2 == nil && err3 == nil
	}, timeout, interval).Should(BeTrue())
}

func validateJobEnvVars(env []corev1.EnvVar, isSource bool) {
	// Validate job env vars
	Expect(len(env)).To(Equal(5))
	validateEnvVar(env, "RCLONE_CONFIG", "/rclone-config/rclone.conf")
	validateEnvVar(env, "RCLONE_DEST_PATH", testRcloneDestPath)
	if isSource {
		validateEnvVar(env, "DIRECTION", "source")
	} else {
		validateEnvVar(env, "DIRECTION", "destination")
	}
	validateEnvVar(env, "MOUNT_PATH", mountPath)
	validateEnvVar(env, "RCLONE_CONFIG_SECTION", testRcloneConfigSection)
}

func validateEnvVar(env []corev1.EnvVar, envVarName, envVarExpectedValue string) {
	found := false
	for _, envVar := range env {
		if envVar.Name == envVarName {
			found = true
			Expect(envVar.Value).To(Equal(envVarExpectedValue))
		}
	}
	Expect(found).To(BeTrue())
}
