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

package rsync

import (
	"flag"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
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
	maxWait  = "60s"
	timeout  = "30s"
	interval = "1s"
)

var _ = Describe("Rsync properly registers", func() {
	When("Rsync's registration function is called", func() {
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

var _ = Describe("Rsync init flags and env vars", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("Rsync builder inits flags", func() {
		var builderForInitTests *Builder
		var testPflagSet *pflag.FlagSet
		BeforeEach(func() {
			os.Unsetenv(rsyncContainerImageEnvVar)

			// Instantiate new viper instance and flagset instance just for this test
			testViper := viper.New()
			testFlagSet := flag.NewFlagSet("testflagset", flag.ExitOnError)

			// New Builder for this test - use testViper and testFlagSet so we can modify
			// flags for these tests without modifying global flags and potentially affecting other tests
			var err error
			builderForInitTests, err = newBuilder(testViper, testFlagSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(builderForInitTests).NotTo(BeNil())

			// code here (see main.go) for viper to bind cmd line flags (including those
			// defined in the mover Register() func)
			// Bind viper to a new set of flags so each of these tests can get their own
			testPflagSet = pflag.NewFlagSet("testpflagset", pflag.ExitOnError)
			testPflagSet.AddGoFlagSet(testFlagSet)
			Expect(testViper.BindPFlags(testPflagSet)).To(Succeed())
		})

		AfterEach(func() {
			os.Unsetenv(rsyncContainerImageEnvVar)
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
					Rsync: &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationSourceStatus{}, // Controller sets status to non-nil
			}
			sourceMover, err := builderForInitTests.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(err).NotTo(HaveOccurred())
			Expect(sourceMover).NotTo(BeNil())
			sourceRsyncMover, _ := sourceMover.(*Mover)
			Expect(sourceRsyncMover.containerImage).To(Equal(builderForInitTests.getRsyncContainerImage()))

			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd",
					Namespace: "testing",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Trigger: &volsyncv1alpha1.ReplicationDestinationTriggerSpec{},
					Rsync:   &volsyncv1alpha1.ReplicationDestinationRsyncSpec{},
				},
				Status: &volsyncv1alpha1.ReplicationDestinationStatus{}, // Controller sets status to non-nil
			}
			destMover, err := builderForInitTests.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(err).NotTo(HaveOccurred())
			Expect(destMover).NotTo(BeNil())
			destRsyncMover, _ := destMover.(*Mover)
			Expect(destRsyncMover.containerImage).To(Equal(builderForInitTests.getRsyncContainerImage()))
		})

		Context("When no command line flag or ENV var is specified", func() {
			It("Should use the default rsync container image", func() {
				Expect(builderForInitTests.getRsyncContainerImage()).To(Equal(defaultRsyncContainerImage))
			})
		})

		Context("When rsync container image command line flag is specified", func() {
			const cmdLineOverrideImageName = "test-rsync-image-name:cmdlineoverride"
			BeforeEach(func() {
				// Manually set the value of the command line flag
				Expect(testPflagSet.Set("rsync-container-image", cmdLineOverrideImageName)).To(Succeed())
			})
			It("Should use the rsync container image set by the cmd line flag", func() {
				Expect(builderForInitTests.getRsyncContainerImage()).To(Equal(cmdLineOverrideImageName))
			})

			Context("And env var is set", func() {
				const envVarOverrideShouldBeIgnored = "test-rsync-image-name:donotuseme"
				BeforeEach(func() {
					os.Setenv(rsyncContainerImageEnvVar, envVarOverrideShouldBeIgnored)
				})
				It("Should still use the cmd line flag instead of the env var", func() {
					Expect(builderForInitTests.getRsyncContainerImage()).To(Equal(cmdLineOverrideImageName))
				})
			})
		})

		Context("When rsync container image cmd line flag is not set and env var is", func() {
			const envVarOverrideImageName = "test-rsync-image-name:setbyenvvar"
			BeforeEach(func() {
				os.Setenv(rsyncContainerImageEnvVar, envVarOverrideImageName)
			})
			It("Should use the value from the env var", func() {
				Expect(builderForInitTests.getRsyncContainerImage()).To(Equal(envVarOverrideImageName))
			})
		})
	})
})

var _ = Describe("Rsync ignores other movers", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	When("An RS isn't for rsync", func() {
		It("is ignored", func() {
			rs := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr",
					Namespace: "blah",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Rsync: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
	When("An RD isn't for rsync", func() {
		It("is ignored", func() {
			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x",
					Namespace: "y",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rsync: nil,
				},
			}
			m, e := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
})

//nolint:goconst
var _ = Describe("Rsync as a source", func() {
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

		// Scaffold ReplicationSource
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rs",
				Namespace: ns.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: sPVC.Name,
				Trigger:   &volsyncv1alpha1.ReplicationSourceTriggerSpec{},
				Rsync:     &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
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

			m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
				true /* privileged */)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("Source volume is handled properly", func() {
			When("CopyMethod is None", func() {
				BeforeEach(func() {
					rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodNone
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
					rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodDirect
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
					rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodClone
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
					rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
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
					// Set snapshot to be bound so the ensureSourcePVC can proceed
					snapshot.Status = &snapv1.VolumeSnapshotStatus{
						BoundVolumeSnapshotContentName: &foo,
					}
					Expect(k8sClient.Status().Update(ctx, &snapshot)).To(Succeed())

					var dataPVC *corev1.PersistentVolumeClaim
					dataPVC, err = mover.ensureSourcePVC(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(dataPVC).NotTo(BeNil())
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

		//nolint:dupl
		Context("Service and address are handled properly", func() {
			When("when no remote address is specified", func() {
				BeforeEach(func() {
					rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
						ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
							CopyMethod: volsyncv1alpha1.CopyMethodClone,
						},
						// No address specified
					}
				})
				It("Creates a Service for incoming connections", func() {
					result, err := mover.ensureServiceAndPublishAddress(ctx)
					Expect(err).To(BeNil())

					// Service should now be created - check to see it's been created
					svc := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-rsync-src-" + rs.Name,
							Namespace: rs.Namespace,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())

					if !result {
						// This means the svc address wasn't populated immediately
						// Keep reconciling - when service has address populated it should get updated in the rs status)
						Eventually(func() bool {
							gotAddr, err := mover.ensureServiceAndPublishAddress(ctx)
							return err != nil && gotAddr
						}, maxWait, interval).Should(BeTrue())

						// Re-Load the service
						err = k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
						Expect(err).ToNot(HaveOccurred())
					}

					Expect(*rs.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))
				})
			})
			When("when a remote address is specified", func() {
				remoteAddr := "testing.remote.host.com"
				BeforeEach(func() {
					rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
						ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
							CopyMethod: volsyncv1alpha1.CopyMethodClone,
						},
						Address: &remoteAddr,
					}
				})
				It("No Service is created", func() {
					// enasureServiecAndPublishAddress should return true,nil immediately
					result, err := mover.ensureServiceAndPublishAddress(ctx)
					Expect(err).To(BeNil())
					Expect(result).To(BeTrue())

					svc := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-rsync-src-" + rs.Name,
							Namespace: rs.Namespace,
						},
					}
					// No service should be created
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc))).To(BeTrue())
				})
			})
		})

		//nolint:dupl
		Context("SSH keys are handled properly", func() {
			When("ssh keys are not specified", func() {
				BeforeEach(func() {
					rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{}
				})
				It("Generates ssh keys automatically", func() {
					var keyName *string
					var err error
					// Call ensureSecrets in eventually as it needs to be called multiple times
					// to create each secret (then expects to be reconciled again to verify)
					Eventually(func() *string {
						keyName, err = mover.ensureSecrets(ctx)
						if err != nil {
							return nil
						}
						return keyName
					}, maxWait, interval).Should(Not(BeNil()))
					Expect(err).To(BeNil())
					Expect(keyName).ToNot(BeNil())

					// No need to reload status from k8s as ensureSecrets updates the status on the rs directly
					// Key name should be the dest key (to be used by the job)
					Expect(*keyName).To(Equal("volsync-rsync-src-src-" + rs.GetName()))
					// Check the correct secret is put in the status (i.e. dest secret for replication source)
					Expect(*rs.Status.Rsync.SSHKeys).To(Equal("volsync-rsync-src-dest-" + rs.GetName()))

					// Check exported secret from status - For replication source this should be a dest secret
					secret1 := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *rs.Status.Rsync.SSHKeys,
						Namespace: rs.Namespace}, secret1)).To(Succeed())
					Expect(secret1.Data).To(HaveKey("destination"))
					Expect(secret1.Data).To(HaveKey("source.pub"))
					Expect(secret1.Data).To(HaveKey("destination.pub"))
					Expect(secret1.Data).NotTo(HaveKey("source"))
					Expect(ownerMatches(secret1, rs.GetName(), true)).To(BeTrue())

					// Check secret that will be mounted by the job
					secret2 := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *keyName,
						Namespace: rs.Namespace}, secret2)).To(Succeed())
					Expect(secret2.Data).To(HaveKey("source"))
					Expect(secret2.Data).To(HaveKey("source.pub"))
					Expect(secret2.Data).To(HaveKey("destination.pub"))
					Expect(secret2.Data).NotTo(HaveKey("destination"))
					Expect(ownerMatches(secret2, rs.GetName(), true)).To(BeTrue())

					// Check that the main secret that contains both source&dest was created
					secret3 := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-main-" + rs.GetName(),
						Namespace: rs.Namespace}, secret3)).To(Succeed())
					Expect(secret3.Data).To(HaveKey("source"))
					Expect(secret3.Data).To(HaveKey("destination"))
					Expect(secret3.Data).To(HaveKey("source.pub"))
					Expect(secret3.Data).To(HaveKey("destination.pub"))
					Expect(ownerMatches(secret3, rs.GetName(), true)).To(BeTrue())
				})
			})

			//nolint:dupl
			Context("When ssh keys are provided", func() {
				Context("When provided secret exists with proper fields", func() {
					var secret *corev1.Secret
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-source-keys",
								Namespace: rs.Namespace,
							},
							StringData: map[string]string{
								"source":          "foo",
								"destination.pub": "bar",
								"source.pub":      "baz",
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
						rs.Spec.Rsync.SSHKeys = &secret.Name // Set ssh keys in the spec
					})
					It("Mover should successfully ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(keyName).NotTo(BeNil())
						Expect(*keyName).To(Equal(secret.GetName()))
					})
				})
				Context("When provided secret exists but missing fields", func() {
					var secret *corev1.Secret
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-source-keys-bad",
								Namespace: rs.Namespace,
							},
							StringData: map[string]string{
								// Missing "source" key
								"destination.pub": "bar",
								"source.pub":      "baz",
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
						rs.Spec.Rsync.SSHKeys = &secret.Name // Set ssh keys in the spec
					})
					It("Mover should fail to ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).To(HaveOccurred())
						Expect(keyName).To(BeNil())
						Expect(err.Error()).To(ContainSubstring("fields"))
						Expect(err.Error()).To(ContainSubstring("source"))
					})
				})
				Context("When provided secret does not exist", func() {
					secretName := "doesnotexist"
					BeforeEach(func() {
						rs.Spec.Rsync.SSHKeys = &secretName
					})
					It("Mover should fail to ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).To(HaveOccurred())
						Expect(keyName).To(BeNil())
						Expect(err.Error()).To(ContainSubstring("not found"))
					})
				})
			})
		})

		Context("ServiceAccount, Role, RoleBinding are handled properly", func() {
			When("Mover is running privileged", func() {
				It("Should create a service account", func() {
					sa, err := mover.saHandler.Reconcile(ctx, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(sa).ToNot(BeNil())

					validateSaRoleAndRoleBinding(sa, ns.GetName())
				})
			})

			When("Mover is unprivileged", func() {
				// This test is here as the common builder func will instantiate rsync (FromSource/Dest)
				// with privileged=false if there is no privileged annotation from the namespace, but
				// rsync is a special case and will always run unprivileged (rsync-tls uses the new behavior
				// and rsync will eventually get deprecated)
				It("Should create a service account with role and role binding with access to scc "+
					"(rsync always runs privileged)", func() {
					// Instantiate a separate rclone mover for this tests using unprivileged
					m, err := commonBuilderForTestSuite.FromSource(k8sClient, logger, &events.FakeRecorder{}, rs,
						false /* unprivileged */)
					Expect(err).NotTo(HaveOccurred())
					Expect(m).NotTo(BeNil())
					unprivMover, _ := m.(*Mover)
					Expect(unprivMover).NotTo(BeNil())

					var sa *corev1.ServiceAccount
					Eventually(func() bool {
						sa, err = unprivMover.saHandler.Reconcile(ctx, logger)
						return sa != nil && err == nil
					}, timeout, interval).Should(BeTrue())
					Expect(err).ToNot(HaveOccurred())

					validateSaRoleAndRoleBinding(sa, ns.GetName())
				})
			})

			//nolint:dupl
			When("A user supplied moverServiceAccount is set in the spec", func() {
				userSuppliedMoverSvcAccount := "cust-svc-acct"
				BeforeEach(func() {
					// Update rsSpec to set our own svc account
					rs.Spec.Rsync.MoverServiceAccount = &userSuppliedMoverSvcAccount
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
		})

		Context("Mover Job is handled properly", func() {
			var jobName string
			var sa *corev1.ServiceAccount
			var sshKeysSecret *corev1.Secret
			var job *batchv1.Job
			var sBlockPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-rsync-src-" + rs.Name

				sa = &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				sshKeysSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testkeys",
						Namespace: rs.Namespace,
					},
					StringData: map[string]string{
						"source":          "foo",
						"source.pub":      "bar",
						"destination.pub": "baz",
					},
				}
			})
			JustBeforeEach(func() {
				blockVolumeMode := corev1.PersistentVolumeBlock
				sc := "spvcsc"
				sBlockPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "blocks",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						VolumeMode: &blockVolumeMode,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("7Gi"),
							},
						},
						StorageClassName: &sc,
					},
				}
				Expect(k8sClient.Create(ctx, sBlockPVC)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, sshKeysSecret)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have the command defined properly", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(Equal(1))
					Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal(
						[]string{"/bin/bash", "-c", "/mover-rsync/source.sh"}))
				})

				It("should use the specified container image", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(defaultRsyncContainerImage))
				})

				It("should use the specified service account", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal(sa.Name))
				})

				It("Should have correct volume mounts", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					c := job.Spec.Template.Spec.Containers[0]
					// Validate job volume mounts
					Expect(len(c.VolumeMounts)).To(Equal(4))
					foundDataVolumeMount := false
					foundSSHSecretVolumeMount := false
					foundDotSSHMount := false
					foundTmpMount := false
					for _, volMount := range c.VolumeMounts {
						if volMount.Name == dataVolumeName {
							foundDataVolumeMount = true
							Expect(volMount.MountPath).To(Equal(mountPath))
						} else if volMount.Name == "keys" {
							foundSSHSecretVolumeMount = true
							Expect(volMount.MountPath).To(Equal("/keys"))
						} else if volMount.Name == "tempsshdir" {
							foundDotSSHMount = true
							Expect(volMount.MountPath).To(Equal("/root/.ssh"))
						} else if volMount.Name == "tempdir" {
							foundTmpMount = true
							Expect(volMount.MountPath).To(Equal("/tmp"))
						}
					}
					Expect(foundDataVolumeMount).To(BeTrue())
					Expect(foundSSHSecretVolumeMount).To(BeTrue())
					Expect(foundDotSSHMount).To(BeTrue())
					Expect(foundTmpMount).To(BeTrue())
				})

				getSPVC := func() *corev1.PersistentVolumeClaim {
					return sPVC
				}

				getBlockPVC := func() *corev1.PersistentVolumeClaim {
					return sBlockPVC
				}

				DescribeTable("Should have correct volumes", func(getPVC func() *corev1.PersistentVolumeClaim) {
					pvc := getPVC()
					Expect(pvc).ToNot(BeNil())
					j, e := mover.ensureJob(ctx, pvc, sa, sshKeysSecret.GetName()) // Using pvc as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					volumes := job.Spec.Template.Spec.Volumes
					Expect(volumes).To(HaveLen(4))
					foundDataVolume := false
					foundSSHSecretVolume := false
					foundDotSSHVolume := false
					foundTmpVolume := false
					for _, vol := range volumes {
						if vol.Name == dataVolumeName {
							foundDataVolume = true
							Expect(vol.VolumeSource.PersistentVolumeClaim).ToNot(BeNil())
							Expect(vol.VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(pvc.GetName()))
							Expect(vol.VolumeSource.PersistentVolumeClaim.ReadOnly).To(Equal(false))
						} else if vol.Name == "keys" {
							foundSSHSecretVolume = true
							Expect(vol.VolumeSource.Secret).ToNot(BeNil())
							Expect(vol.VolumeSource.Secret.SecretName).To(Equal(sshKeysSecret.GetName()))
						} else if vol.Name == "tempsshdir" {
							foundDotSSHVolume = true
							Expect(vol.VolumeSource.EmptyDir).ToNot(BeNil())
						} else if vol.Name == "tempdir" {
							foundTmpVolume = true
							Expect(vol.VolumeSource.EmptyDir).ToNot(BeNil())
						}
					}
					Expect(foundDataVolume).To(BeTrue())
					Expect(foundSSHSecretVolume).To(BeTrue())
					Expect(foundDotSSHVolume).To(BeTrue())
					Expect(foundTmpVolume).To(BeTrue())
				},
					Entry("Filesystem volume", getSPVC),
					Entry("Block volume", getBlockPVC),
				)
				When("The source PVC has volumeMode: block", func() {
					It("Should have correct volume mounts, and device mount", func() {
						j, e := mover.ensureJob(ctx, sBlockPVC, sa, sshKeysSecret.GetName()) // Using sBlockPVC as dataPVC (i.e. direct)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
						job = &batchv1.Job{}
						Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

						c := job.Spec.Template.Spec.Containers[0]
						// Validate job volume mounts
						Expect(len(c.VolumeMounts)).To(Equal(3))
						foundDataVolumeMount := false
						foundSSHSecretVolumeMount := false
						foundDotSSHMount := false
						foundTmpMount := false
						for _, volMount := range c.VolumeMounts {
							if volMount.Name == dataVolumeName {
								foundDataVolumeMount = true
								Expect(volMount.MountPath).To(Equal(mountPath))
							} else if volMount.Name == "keys" {
								foundSSHSecretVolumeMount = true
								Expect(volMount.MountPath).To(Equal("/keys"))
							} else if volMount.Name == "tempsshdir" {
								foundDotSSHMount = true
								Expect(volMount.MountPath).To(Equal("/root/.ssh"))
							} else if volMount.Name == "tempdir" {
								foundTmpMount = true
								Expect(volMount.MountPath).To(Equal("/tmp"))
							}
						}
						Expect(foundDataVolumeMount).To(BeFalse())
						Expect(foundSSHSecretVolumeMount).To(BeTrue())
						Expect(foundDotSSHMount).To(BeTrue())
						Expect(foundTmpMount).To(BeTrue())

						Expect(c.VolumeDevices).To(HaveLen(1))
						foundDataVolumeDevice := false
						for _, volDevice := range c.VolumeDevices {
							if volDevice.Name == dataVolumeName {
								foundDataVolumeDevice = true
								Expect(volDevice.DevicePath).To(Equal("/dev/block"))
							}
						}
						Expect(foundDataVolumeDevice).To(BeTrue())
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
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										"storage": resource.MustParse("1Gi"),
									},
								},
							},
						}
						Expect(k8sClient.Create(ctx, roxPVC)).To(Succeed())
					})
					It("Mover job should mount the PVC as read-only", func() {
						j, e := mover.ensureJob(ctx, roxPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
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

				It("Should have correct labels", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// It should be marked for cleaned up
					Expect(job.Labels).To(HaveKey("volsync.backube/cleanup"))
				})

				It("should support pausing", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(1)))

					mover.paused = true
					j, e = mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(0)))

					mover.paused = false
					j, e = mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(*job.Spec.Parallelism).To(Equal(int32(1)))
				})

				It("should have no env vars when no address is set in spec", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Validate job env vars
					env := job.Spec.Template.Spec.Containers[0].Env
					Expect(len(env)).To(Equal(0))
				})
			})

			When("initial sync and address is specified in rsync spec", func() {
				var address string
				BeforeEach(func() {
					// Set an address in the spec but no port
					address = "https://testserver.mydomain:8888"
					rs.Spec.Rsync.Address = &address
				})
				It("should have the correct env vars when address is set in spec", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Validate job env vars
					env := job.Spec.Template.Spec.Containers[0].Env
					Expect(len(env)).To(Equal(1))
					validateEnvVar(env, "DESTINATION_ADDRESS", address)
				})
			})

			When("initial sync and address and port are specified in rsync spec", func() {
				var address string
				var port int32
				BeforeEach(func() {
					// Set an address in the spec but no port
					address = "https://testserver.mydomain:8888"
					rs.Spec.Rsync.Address = &address

					port = 4567
					rs.Spec.Rsync.Port = &port
				})
				It("should have the correct env vars when address is set in spec", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Validate job env vars
					env := job.Spec.Template.Spec.Containers[0].Env
					Expect(len(env)).To(Equal(2))
					validateEnvVar(env, "DESTINATION_ADDRESS", address)
					validateEnvVar(env, "DESTINATION_PORT", strconv.Itoa(int(port)))
				})
			})

			When("Doing a sync when the job already exists", func() {
				JustBeforeEach(func() {
					mover.containerImage = "my-rsync-mover-image"

					// Initial job creation
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed

					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(mover.containerImage))
				})

				It("Should recreate the job if job.spec.template needs modification", func() {
					myUpdatedImage := "somenew-rsync-mover:latest"

					// change to simulate mover image being updated
					mover.containerImage = myUpdatedImage

					// Mover should get immutable err for updating the image and then delete the job
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).To(HaveOccurred())
					Expect(j).To(BeNil())

					// Make sure job has been deleted
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, nsn, job))).To(BeTrue())

					// Run ensureJob again as the reconciler would do - should recreate the job
					j, e = mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // job hasn't completed

					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(myUpdatedImage))
				})
			})

			When("the job has failed", func() {
				It("should be restarted", func() {
					j, e := mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Set the job status to have failed backofflimit times
					job.Status.Failed = *job.Spec.BackoffLimit
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

					// Since job is failed >= backofflimit, ensureJob should remove the job so it can be recreated
					j, e = mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil())
					// Job should be deleted
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, nsn, job))).To(BeTrue())

					// Reconcile again, job should get recreated on next call to ensureJob
					j, e = mover.ensureJob(ctx, sPVC, sa, sshKeysSecret.GetName()) // Using sPVC as dataPVC (i.e. direct)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // will return nil since job is not completed

					// Job should now be recreated
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(job.Status.Failed).Should(Equal(int32(0)))
				})
			})
		})
	})
})

//nolint:goconst
var _ = Describe("Rsync as a destination", func() {
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
				Rsync:   &volsyncv1alpha1.ReplicationDestinationRsyncSpec{},
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

			m, err := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
				true /* privileged */)
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
					rd.Spec.Rsync.AccessModes = []corev1.PersistentVolumeAccessMode{
						am,
					}
					cap = resource.MustParse("6Gi")
					rd.Spec.Rsync.Capacity = &cap
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
			createPVCSpec := func(volumeMode corev1.PersistentVolumeMode) *corev1.PersistentVolumeClaim {
				return &corev1.PersistentVolumeClaim{
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
						VolumeMode: &volumeMode,
					},
				}
			}

			When("a destination volume is supplied", func() {
				var dPVC *corev1.PersistentVolumeClaim
				BeforeEach(func() {
					dPVC = createPVCSpec(corev1.PersistentVolumeFilesystem)
					Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
					rd.Spec.Rsync.DestinationPVC = &dPVC.Name
				})
				It("is used directly", func() {
					pvc, e := mover.ensureDestinationPVC(ctx)
					Expect(e).NotTo(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					Expect(pvc.Name).To(Equal(dPVC.Name))
					// It's not owned by the CR
					Expect(pvc.OwnerReferences).To(BeEmpty())
					// It won't be cleaned up at the end of the transfer
					Expect(pvc.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
			When("a block destination volume is supplied", func() {
				var dPVC *corev1.PersistentVolumeClaim
				BeforeEach(func() {
					dPVC = createPVCSpec(corev1.PersistentVolumeBlock)
					Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
					rd.Spec.Rsync.DestinationPVC = &dPVC.Name
				})
				It("is used directly", func() {
					pvc, e := mover.ensureDestinationPVC(ctx)
					Expect(e).NotTo(HaveOccurred())
					Expect(pvc).NotTo(BeNil())
					Expect(pvc.Name).To(Equal(dPVC.Name))
					// It's not owned by the CR
					Expect(pvc.OwnerReferences).To(BeEmpty())
					// It won't be cleaned up at the end of the transfer
					Expect(pvc.Labels).NotTo(HaveKey("volsync.backube/cleanup"))
				})
			})
		})

		//nolint:dupl
		Context("Service and address are handled properly", func() {
			When("when no remote address is specified", func() {
				var svc *corev1.Service
				JustBeforeEach(func() {
					// create the svc
					result, err := mover.ensureServiceAndPublishAddress(ctx)
					Expect(err).To(BeNil())

					// Service should now be created - check to see it's been created
					svc = &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-rsync-dst-" + rd.Name,
							Namespace: rd.Namespace,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())

					if !result {
						// This means the svc address wasn't populated immediately
						// Keep reconciling - when service has address populated it should get updated in the rs status)
						Eventually(func() bool {
							gotAddr, err := mover.ensureServiceAndPublishAddress(ctx)
							return err != nil && gotAddr
						}, maxWait, interval).Should(BeTrue())

						// Re-Load the service
						err = k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
						Expect(err).ToNot(HaveOccurred())
					}
				})

				When("spec leaves service defaults", func() {
					BeforeEach(func() {
						rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
							// No address specified
							// No serviceAnnotations specified
						}
					})
					It("Creates a Service for incoming connections with defaults", func() {
						Expect(*rd.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))

						// Check for default annotation VolSync adds
						defaultAnnotation, ok := svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"]
						Expect(ok).To(BeTrue())
						Expect(defaultAnnotation).To(Equal("nlb"))

						// Doublecheck here that the rd should have nil serviceAnnotations set after re-loading
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).NotTo(HaveOccurred())
						Expect(rd.Spec.Rsync.ServiceAnnotations).To(BeNil())
					})
				})

				When("spec has empty serviceAnnotations", func() {
					BeforeEach(func() {
						// Empty serviceAnnnotations (i.e. {}) should be treated differently from not specifying serviceAnnotations.
						// In the empty case we want to override any annotations VolSync might set by default
						rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
							ServiceAnnotations: &map[string]string{},
						}
					})
					It("Creates a Service for incoming connections with no volsync annotations", func() {
						Expect(*rd.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))

						Expect(len(svc.Annotations)).To(Equal(0))

						// Doublecheck here that the rd should have empty serviceAnnotations set after re-loading
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).NotTo(HaveOccurred())
						Expect(rd.Spec.Rsync.ServiceAnnotations).NotTo(BeNil())
						Expect(*rd.Spec.Rsync.ServiceAnnotations).To(Equal(map[string]string{}))
					})
				})

				When("spec has serviceAnnotations", func() {
					myCustAnnotations := map[string]string{
						"custom-svc-annotation1": "apples",
						"custom-svc-annotation2": "oranges",
					}

					BeforeEach(func() {
						rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
							ServiceAnnotations: &myCustAnnotations,
						}
					})
					It("Creates a Service for incoming connections with no volsync annotations", func() {
						Expect(*rd.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))

						Expect(svc.Annotations).To(Equal(myCustAnnotations))
					})
				})
			})
			When("when a remote address is specified", func() {
				remoteAddr := "abcd.test.org"
				BeforeEach(func() {
					rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
						Address: &remoteAddr,
					}
				})
				It("No Service is created", func() {
					// enasureServiecAndPublishAddress should return true,nil immediately
					result, err := mover.ensureServiceAndPublishAddress(ctx)
					Expect(err).To(BeNil())
					Expect(result).To(BeTrue())

					svc := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "volsync-rsync-dst-" + rd.Name,
							Namespace: rd.Namespace,
						},
					}
					Expect(kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc))).To(BeTrue())
				})
			})
		})

		//nolint:dupl
		Context("SSH keys are handled properly", func() {
			When("ssh keys are not specified", func() {
				BeforeEach(func() {
					rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{}
				})
				It("Generates ssh keys automatically", func() {
					var keyName *string
					var err error
					// Call ensureSecrets in eventually as it needs to be called multiple times
					// to create each secret (then expects to be reconciled again to verify)
					Eventually(func() *string {
						keyName, err = mover.ensureSecrets(ctx)
						if err != nil {
							return nil
						}
						return keyName
					}, maxWait, interval).Should(Not(BeNil()))
					Expect(err).To(BeNil())
					Expect(keyName).ToNot(BeNil())

					// No need to reload status from k8s as ensureSecrets updates the status on the rd directly
					// Key name should be the dest key (to be used by the job)
					Expect(*keyName).To(Equal("volsync-rsync-dst-dest-" + rd.GetName()))
					// Check the correct secret is put in the status (i.e. exported src secret for replication destination)
					Expect(*rd.Status.Rsync.SSHKeys).To(Equal("volsync-rsync-dst-src-" + rd.GetName()))

					secret := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *rd.Status.Rsync.SSHKeys,
						Namespace: rd.Namespace}, secret)).To(Succeed())
					Expect(secret.Data).To(HaveKey("source"))
					Expect(secret.Data).To(HaveKey("source.pub"))
					Expect(secret.Data).To(HaveKey("destination.pub"))
					Expect(secret.Data).NotTo(HaveKey("destination"))
					Expect(ownerMatches(secret, rd.GetName(), false)).To(BeTrue())

					// Check secret that will be mounted by the job
					secret2 := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *keyName,
						Namespace: rd.Namespace}, secret2)).To(Succeed())
					Expect(secret2.Data).To(HaveKey("destination"))
					Expect(secret2.Data).To(HaveKey("source.pub"))
					Expect(secret2.Data).To(HaveKey("destination.pub"))
					Expect(secret2.Data).NotTo(HaveKey("source"))
					Expect(ownerMatches(secret2, rd.GetName(), false)).To(BeTrue())

					// Check that the main secret that contains both source&dest was created
					secret3 := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-main-" + rd.GetName(),
						Namespace: rd.Namespace}, secret3)).To(Succeed())
					Expect(secret3.Data).To(HaveKey("source"))
					Expect(secret3.Data).To(HaveKey("destination"))
					Expect(secret3.Data).To(HaveKey("source.pub"))
					Expect(secret3.Data).To(HaveKey("destination.pub"))
					Expect(ownerMatches(secret3, rd.GetName(), false)).To(BeTrue())
				})
			})

			//nolint:dupl
			Context("When ssh keys are provided", func() {
				Context("When provided secret exists with proper fields", func() {
					var secret *corev1.Secret
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-dest-keys",
								Namespace: rd.Namespace,
							},
							StringData: map[string]string{
								"destination":     "foo",
								"destination.pub": "bar",
								"source.pub":      "baz",
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
						rd.Spec.Rsync.SSHKeys = &secret.Name // Set ssh keys in the spec
					})
					It("Mover should successfully ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(keyName).NotTo(BeNil())
						Expect(*keyName).To(Equal(secret.GetName()))
					})
				})
				Context("When provided secret exists but missing fields", func() {
					var secret *corev1.Secret
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-dest-keys-bad",
								Namespace: rd.Namespace,
							},
							StringData: map[string]string{
								// Missing "destination" key
								"destination.pub": "bar",
								"source.pub":      "baz",
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
						rd.Spec.Rsync.SSHKeys = &secret.Name // Set ssh keys in the spec
					})
					It("Mover should fail to ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).To(HaveOccurred())
						Expect(keyName).To(BeNil())
						Expect(err.Error()).To(ContainSubstring("fields"))
						Expect(err.Error()).To(ContainSubstring("destination"))
					})
				})
				Context("When provided secret does not exist", func() {
					secretName := "doesnotexist"
					BeforeEach(func() {
						rd.Spec.Rsync.SSHKeys = &secretName
					})
					It("Mover should fail to ensureSecrets", func() {
						keyName, err := mover.ensureSecrets(ctx)
						Expect(err).To(HaveOccurred())
						Expect(keyName).To(BeNil())
						Expect(err.Error()).To(ContainSubstring("not found"))
					})
				})
			})
		})

		//nolint:dupl
		Context("ServiceAccount, Role, RoleBinding are handled properly", func() {
			When("Mover is running privileged", func() {
				It("Should create a service account with role that allows access to the scc", func() {
					var sa *corev1.ServiceAccount
					var err error
					Eventually(func() bool {
						sa, err = mover.saHandler.Reconcile(ctx, logger)
						return sa != nil && err == nil
					}, timeout, interval).Should(BeTrue())
					Expect(err).ToNot(HaveOccurred())

					validateSaRoleAndRoleBinding(sa, ns.GetName())
				})
			})

			When("Mover is unprivileged", func() {
				// This test is here as the common builder func will instantiate rsync (FromSource/Dest)
				// with privileged=false if there is no privileged annotation from the namespace, but
				// rsync is a special case and will always run unprivileged (rsync-tls uses the new behavior
				// and rsync will eventually get deprecated)
				It("Should create a service account with role and role binding with access to scc "+
					"(rsync always runs privileged)", func() {
					// Instantiate a separate rclone mover for this tests using unprivileged
					m, err := commonBuilderForTestSuite.FromDestination(k8sClient, logger, &events.FakeRecorder{}, rd,
						false /* unprivileged */)
					Expect(err).NotTo(HaveOccurred())
					Expect(m).NotTo(BeNil())
					unprivMover, _ := m.(*Mover)
					Expect(unprivMover).NotTo(BeNil())

					var sa *corev1.ServiceAccount
					Eventually(func() bool {
						sa, err = unprivMover.saHandler.Reconcile(ctx, logger)
						return sa != nil && err == nil
					}, timeout, interval).Should(BeTrue())
					Expect(err).ToNot(HaveOccurred())

					validateSaRoleAndRoleBinding(sa, ns.GetName())
				})
			})

			When("A user supplied moverServiceAccount is set in the spec", func() {
				userSuppliedMoverSvcAccount := "cust-svc-acct"
				BeforeEach(func() {
					// Update rsSpec to set our own svc account
					rd.Spec.Rsync.MoverServiceAccount = &userSuppliedMoverSvcAccount
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
		})
		Context("Mover Job is handled properly", func() {
			var jobName string
			var dPVC *corev1.PersistentVolumeClaim
			var sa *corev1.ServiceAccount
			var job *batchv1.Job
			testSSSKey := "secretNameHere"
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-rsync-dst-" + rd.Name
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
			})
			JustBeforeEach(func() {
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have the command defined properly", func() {
					j, e := mover.ensureJob(ctx, dPVC, sa, testSSSKey)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(Equal(1))
					Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal(
						[]string{"/bin/bash", "-c", "/mover-rsync/destination.sh"}))
				})
				It("should have the correct env vars", func() {
					j, e := mover.ensureJob(ctx, dPVC, sa, testSSSKey)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Expect(k8sClient.Get(ctx, nsn, job)).To(Succeed())

					// Validate job env vars - no env vars on dest job
					Expect(len(job.Spec.Template.Spec.Containers[0].Env)).To(Equal(0))
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
				rd.Spec.Rsync.DestinationPVC = &dPVC.Name

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dPVC), dPVC)).To(Succeed())
				// Update this PVC manually for the test to put the snapshot annotation on it
				if dPVC.Annotations == nil {
					dPVC.Annotations = make(map[string]string)
				}
				dPVC.Annotations["volsync.backube/snapname"] = "testisafakesnapshotannoation"
				Expect(k8sClient.Update(ctx, dPVC)).To(Succeed())

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

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snap1), snap1)).To(Succeed())
				_, ok := snap1.GetLabels()["volsync.backube/cleanup"]
				Expect(ok).To(BeTrue())

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
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dPVC), dPVC)).To(Succeed())
				Expect(dPVC.GetAnnotations()["volsync.backube/snap"])
				Expect(dPVC.Annotations).ToNot(HaveKey("volsync.backube/snapname"))
			})
			It("Should remove old snapshot", func() {
				result, err := mover.Cleanup(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Completed).To(BeTrue())

				snapshots := &snapv1.VolumeSnapshotList{}
				Expect(k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))).To(Succeed())
				Expect(len(snapshots.Items)).Should(Equal(1))
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
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sa), sa)).To(Succeed())
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(role), role)).To(Succeed())
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(roleBinding), roleBinding)).To(Succeed())

	// Check to make sure the role grants access to the privileged mover scc
	// rsync should always run privileged
	Expect(len(role.Rules)).To(Equal(1))
	rule := role.Rules[0]
	Expect(rule.APIGroups).To(Equal([]string{"security.openshift.io"}))
	Expect(rule.Resources).To(Equal([]string{"securitycontextconstraints"}))
	Expect(rule.ResourceNames).To(Equal([]string{utils.SCCName}))
	Expect(rule.Verbs).To(Equal([]string{"use"}))
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

func ownerMatches(obj metav1.Object, ownerName string, isSource bool) bool {
	kind := "ReplicationDestination"
	if isSource {
		kind = "ReplicationSource"
	}
	foundOwner := false
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.Name == ownerName && ownerRef.Kind == kind {
			foundOwner = true
		}
	}
	return foundOwner
}
