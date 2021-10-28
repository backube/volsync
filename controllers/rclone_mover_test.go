package controllers

import (
	"context"
	"fmt"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

//nolint:dupl
var _ = Describe("ReplicationDestination [rclone]", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rd *volsyncv1alpha1.ReplicationDestination
	var rcloneSecret *corev1.Secret
	var configSection = "foo"
	var destPath = "bar"
	var pvc *corev1.PersistentVolumeClaim
	var job *batchv1.Job
	var schedule = "*/4 * * * *"
	var capacity = resource.MustParse("2Gi")

	directCopyMethodTypes := []volsyncv1alpha1.CopyMethodType{
		volsyncv1alpha1.CopyMethodNone,
		volsyncv1alpha1.CopyMethodDirect,
	}

	// setup namespace && PVC
	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-rclone-dest-",
			},
		}
		// crete ns
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())

		// sets up RD, Rclone, Secret & PVC spec
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"storage": resource.MustParse("2Gi"),
					},
				},
			},
		}
		rcloneSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rclone-secret",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"rclone.conf": "hunter2",
			},
		}
		// scaffolded ReplicationDestination - extra fields will be set in subsequent tests
		rd = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
		}
		// setup a minimal job
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "volsync-rclone-src-" + rd.Name,
				Namespace: rd.Namespace,
			},
		}
		RcloneContainerImage = DefaultRcloneContainerImage
	})
	AfterEach(func() {
		// delete each namespace on shutdown so resources can be reclaimed
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// create necessary resources
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
		Expect(k8sClient.Create(ctx, rcloneSecret)).To(Succeed())
		Expect(k8sClient.Create(ctx, rd)).To(Succeed())
		// wait for the ReplicationDestination to actually come up
		Eventually(func() error {
			inst := &volsyncv1alpha1.ReplicationDestination{}
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
		}, maxWait, interval).Should(Succeed())
	})

	//nolint:dupl
	When("ReplicationDestination is provided with a minimal rclone spec", func() {
		BeforeEach(func() {
			rd.Spec.Rclone = &volsyncv1alpha1.ReplicationDestinationRcloneSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{},
				RcloneConfigSection:                 &configSection,
				RcloneDestPath:                      &destPath,
				RcloneConfig:                        &rcloneSecret.Name,
			}
			rd.Spec.Trigger = &volsyncv1alpha1.ReplicationDestinationTriggerSpec{
				Schedule: &schedule,
			}
		})

		It("should start", func() {
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
			}, duration, interval).Should(Succeed())
		})

		When("ReplicationDestination is provided AccessModes & Capacity", func() {
			BeforeEach(func() {
				rd.Spec.Rclone.ReplicationDestinationVolumeOptions = volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Capacity:    &capacity,
				}
			})
			When("Job Status is set to succeeded", func() {
				JustBeforeEach(func() {
					// force job to succeed
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, maxWait, interval).Should(Succeed())
					job.Status.Succeeded = 1
					job.Status.StartTime = &metav1.Time{ // provide job with a start time to get a duration
						Time: time.Now(),
					}
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
				})

				// Test both None and Direct (results should be the same)
				for i := range directCopyMethodTypes {
					When(fmt.Sprintf("Using a CopyMethod of %s", directCopyMethodTypes[i]), func() {
						directCopyMethodType := directCopyMethodTypes[i]
						BeforeEach(func() {
							rd.Spec.Rclone.CopyMethod = directCopyMethodType
						})

						It("Ensure that ReplicationDestination starts", func() {
							// get RD & ensure a status is set
							Eventually(func() bool {
								inst := &volsyncv1alpha1.ReplicationDestination{}
								if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst); err != nil {
									return false
								}
								return inst.Status != nil
							}, maxWait, interval).Should(BeTrue())
							// validate necessary fields
							inst := &volsyncv1alpha1.ReplicationDestination{}
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)).To(Succeed())
							Expect(inst.Status).NotTo(BeNil())
							Expect(inst.Status.Conditions).NotTo(BeNil())
							Expect(inst.Status.NextSyncTime).NotTo(BeNil())
						})

						It("Ensure LastSyncTime & LatestImage is set properly after reconciliation", func() {
							Eventually(func() bool {
								inst := &volsyncv1alpha1.ReplicationDestination{}
								err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
								if err != nil || inst.Status == nil {
									return false
								}
								return inst.Status.LastSyncTime != nil
							}, maxWait, interval).Should(BeTrue())
							// get ReplicationDestination
							inst := &volsyncv1alpha1.ReplicationDestination{}
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)).To(Succeed())
							// ensure Status holds correct data
							Expect(inst.Status.LatestImage).NotTo(BeNil())
							latestImage := inst.Status.LatestImage
							Expect(latestImage.Kind).To(Equal("PersistentVolumeClaim"))
							Expect(*latestImage.APIGroup).To(Equal(""))
							Expect(latestImage.Name).NotTo(Equal(""))
						})

						It("Duration is set if job is successful", func() {
							// Make sure that LastSyncDuration gets set
							Eventually(func() *metav1.Duration {
								inst := &volsyncv1alpha1.ReplicationDestination{}
								_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
								return inst.Status.LastSyncDuration
							}, maxWait, interval).Should(Not(BeNil()))
						})

						When("The ReplicationDestinaton spec is paused", func() {
							parallelism := int32(0)
							BeforeEach(func() {
								// set paused to true so no more processes will be created
								rd.Spec.Paused = true
							})
							It("Job has parallelism disabled", func() {
								job := &batchv1.Job{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "volsync-rclone-src-" + rd.Name,
										Namespace: rd.Namespace,
									},
								}
								Eventually(func() error {
									return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
								}, maxWait, interval).Should(Succeed())
								Expect(*job.Spec.Parallelism).To(Equal(parallelism))
							})
						})

						When("A Storage Class is specified", func() {
							scName := "mysc"
							BeforeEach(func() {
								rd.Spec.Rclone.ReplicationDestinationVolumeOptions.StorageClassName = &scName
							})
							It("Is used in the destination PVC", func() {
								// gets the pvcs used by the job spec
								Eventually(func() error {
									return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
								}).Should(Succeed())
								var pvcName string
								volumes := job.Spec.Template.Spec.Volumes
								for _, v := range volumes {
									if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
										pvcName = v.PersistentVolumeClaim.ClaimName
									}
								}
								pvc = &corev1.PersistentVolumeClaim{}
								Eventually(func() error {
									return k8sClient.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: rd.Namespace}, pvc)
								}, maxWait, interval).Should(Succeed())
								Expect(*pvc.Spec.StorageClassName).To(Equal(scName))
							})
						})
					})
				}

				When("Using a copy method of Snapshot", func() {
					BeforeEach(func() {
						rd.Spec.Rclone.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
					})

					It("Ensure that a VolumeSnapshot is created at the end of an iteration", func() {
						// get list of volume snapshots in the ns
						snapshots := &snapv1.VolumeSnapshotList{}
						Eventually(func() []snapv1.VolumeSnapshot {
							_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
							return snapshots.Items
						}, maxWait, interval).Should(Not(BeEmpty()))
						// update the VS name
						snapshot := snapshots.Items[0]
						foo := "dummysnapshot"
						snapshot.Status = &snapv1.VolumeSnapshotStatus{
							BoundVolumeSnapshotContentName: &foo,
						}
						Expect(k8sClient.Status().Update(ctx, &snapshot)).To(Succeed())
						// wait for an image to be set for RD
						Eventually(func() *corev1.TypedLocalObjectReference {
							_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							return rd.Status.LatestImage
						}, maxWait, interval).Should(Not(BeNil()))
						latestImage := rd.Status.LatestImage
						// ensure the name was correctly set
						Expect(latestImage.Kind).To(Equal("VolumeSnapshot"))
						Expect(*latestImage.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
						Expect(latestImage).To(Not(Equal("")))
					})

					When("When a VolumeSnapshotClass is specified", func() {
						vscName := "MyVolumeSnapshotClass"
						BeforeEach(func() {
							rd.Spec.Rclone.ReplicationDestinationVolumeOptions.VolumeSnapshotClassName = &vscName
						})

						It("is used as the VSC for the Snapshot", func() {
							// create a snapshot & verify the VSC matches
							snapshots := &snapv1.VolumeSnapshotList{}
							Eventually(func() []snapv1.VolumeSnapshot {
								_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
								return snapshots.Items
							}, maxWait, interval).Should(Not(BeEmpty()))
							snapshot := snapshots.Items[0]
							Expect(*snapshot.Spec.VolumeSnapshotClassName).To(Equal(vscName))
						})
					})
				})
			})
			It("Job set to fail", func() {
				tempJob := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), tempJob)
				}, maxWait, interval).Should(Succeed())
				// force job fail state
				tempJob.Status.Failed = *tempJob.Spec.BackoffLimit + 12345
				Expect(k8sClient.Status().Update(ctx, tempJob)).To(Succeed())
				Expect(tempJob.Status.Failed).NotTo(BeNumerically("==", 0))
				// ensure job eventually gets restarted
				Eventually(func() bool {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), tempJob); err != nil {
						return false
					}
					return tempJob.Status.Failed == 0
				}, maxWait, interval).Should(BeTrue())
			})
		})
	})

	When("Secret has incorrect values", func() {
		Context("Secret isn't provided incorrect fields", func() {
			BeforeEach(func() {
				// setup a sus secret
				rcloneSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rclone-secret",
						Namespace: namespace.Name,
					},
					StringData: map[string]string{
						"field-redacted": "this data is trash",
					},
				}
				// valid rclone spec
				rd.Spec.Rclone = &volsyncv1alpha1.ReplicationDestinationRcloneSpec{
					ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Capacity:    &capacity,
					},
					RcloneConfigSection: &configSection,
					RcloneDestPath:      &destPath,
					RcloneConfig:        &rcloneSecret.Name,
				}
			})
			It("Reconcile Condition is set to false", func() {
				// make sure that the condition is set to not be reconciled
				inst := &volsyncv1alpha1.ReplicationDestination{}
				// wait for replicationdestination to have a status
				Eventually(func() *volsyncv1alpha1.ReplicationDestinationStatus {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
					return inst.Status
				}, duration, interval).Should(Not(BeNil()))
				Expect(inst.Status.Conditions).ToNot(BeEmpty())
				reconcileCondition := apimeta.FindStatusCondition(inst.Status.Conditions, volsyncv1alpha1.ConditionReconciled)
				Expect(reconcileCondition).ToNot(BeNil())
				Expect(reconcileCondition.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		// test each of the possible configurations
		Context("Secret fields are zero-length", func() {
			BeforeEach(func() {
				// initialize all config sections to zero length
				var zeroLength = ""
				rd.Spec.Rclone = &volsyncv1alpha1.ReplicationDestinationRcloneSpec{
					ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Capacity:    &capacity,
					},
					RcloneConfigSection: &zeroLength,
					RcloneDestPath:      &zeroLength,
					RcloneConfig:        &zeroLength,
				}
				rd.Spec.Trigger = &volsyncv1alpha1.ReplicationDestinationTriggerSpec{
					Schedule: &schedule,
				}
				// setup a minimal job
				job = &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-rclone-src-" + rd.Name,
						Namespace: rd.Namespace,
					},
				}
			})
			When("All of the fields are zero-length", func() {
				It("Job fails to start", func() {
					Consistently(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, time.Second, interval).ShouldNot(Succeed())
				})
			})

			When("Some of the config sections are provided", func() {
				Context("RcloneConfig", func() {
					BeforeEach(func() {
						rd.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
					})
					It("RcloneConfig is provided", func() {
						Consistently(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, time.Second, interval).ShouldNot(Succeed())
					})
				})
				Context("RcloneConfig + RcloneConfigSection", func() {
					BeforeEach(func() {
						rd.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
						rd.Spec.Rclone.RcloneConfigSection = &configSection
					})
					It("RcloneConfig & RcloneConfigSection are set to non-nil", func() {
						Consistently(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, time.Second, interval).ShouldNot(Succeed())
					})
				})
				Context("Everything is provided", func() {
					BeforeEach(func() {
						rd.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
						rd.Spec.Rclone.RcloneConfigSection = &configSection
						rd.Spec.Rclone.RcloneDestPath = &destPath
					})
					It("Job successfully starts", func() {
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, maxWait, interval).Should(Succeed())
					})
				})
			})
		})
	})
})

//nolint:dupl
var _ = Describe("ReplicationSource [rclone]", func() {
	var namespace *corev1.Namespace
	var rs *volsyncv1alpha1.ReplicationSource
	var srcPVC *corev1.PersistentVolumeClaim
	var job *batchv1.Job
	var rcloneSecret *corev1.Secret
	var ctx = context.Background()
	var srcPVCCapacity = resource.MustParse("7Gi")
	var configSection = "foo"
	var destPath = "bar"

	directCopyMethodTypes := []volsyncv1alpha1.CopyMethodType{
		volsyncv1alpha1.CopyMethodNone,
		volsyncv1alpha1.CopyMethodDirect,
	}

	// setup namespace && PVC
	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-rclone-test-",
			},
		}
		// crete ns
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())
		srcPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thesource",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: srcPVCCapacity,
					},
				},
			},
		}
		// need secret for most of these tests to work
		rcloneSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rclone-secret",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"rclone.conf": "hunter2",
			},
		}
		// baseline spec
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: srcPVC.Name,
			},
		}
		// minimal job
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "volsync-rclone-src-" + rs.Name,
				Namespace: namespace.Name,
			},
		}
		RcloneContainerImage = DefaultRcloneContainerImage
	})
	AfterEach(func() {
		// delete each namespace on shutdown so resources can be reclaimed
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// at least the srcPVC & rcloneSecret should be expected to start at this point
		Expect(k8sClient.Create(ctx, srcPVC)).To(Succeed())
		Expect(k8sClient.Create(ctx, rcloneSecret)).To(Succeed())
	})
	When("Components are expected to start", func() {
		JustBeforeEach(func() {
			// source pvc comes up
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			// wait for the ReplicationSource to actually come up
			Eventually(func() error {
				inst := &volsyncv1alpha1.ReplicationSource{}
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), inst)
			}, maxWait, interval).Should(Succeed())
		})

		When("ReplicationSource is provided with an Rclone spec", func() {
			BeforeEach(func() {
				rs.Spec.Rclone = &volsyncv1alpha1.ReplicationSourceRcloneSpec{
					ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{},
					RcloneConfigSection:            &configSection,
					RcloneDestPath:                 &destPath,
					RcloneConfig:                   &rcloneSecret.Name,
				}
			})
			When("The Job Succeeds", func() {
				JustBeforeEach(func() {
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, maxWait, interval).Should(Succeed())
					// just so the tests will run for now
					job.Status.Succeeded = 1
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
				})

				// Test both None and Direct (results should be the same)
				for i := range directCopyMethodTypes {
					When(fmt.Sprintf("copyMethod of %s is specified for Rclone", directCopyMethodTypes[i]), func() {
						directCopyMethodType := directCopyMethodTypes[i]
						BeforeEach(func() {
							rs.Spec.Rclone.ReplicationSourceVolumeOptions.CopyMethod = directCopyMethodType
						})

						When("No schedule is provided to ReplicationSource", func() {
							It("NextSyncTime is never set", func() {
								Consistently(func() bool {
									// replication source should exist within k8s cluster
									Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
									if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
										return false
									}
									return true
								}, duration, interval).Should(BeFalse())
							})
						})

						When("Schedule is provided", func() {
							var schedule string
							When("Schedule is a proper cron format", func() {
								BeforeEach(func() {
									schedule = "1 3 3 7 *"
									rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
										Schedule: &schedule,
									}
								})
								It("the next sync time is set in Status.NextSyncTime", func() {
									Eventually(func() bool {
										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
										if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
											return false
										}
										return true
									}, maxWait, interval).Should(BeTrue())
								})
							})
						})
					})
				}

				When("copyMethod of Clone is specified", func() {
					BeforeEach(func() {
						rs.Spec.Rclone.ReplicationSourceVolumeOptions.CopyMethod = volsyncv1alpha1.CopyMethodClone
					})

					//nolint:dupl
					It("creates a clone of the source PVC as the sync source", func() {
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, maxWait, interval).Should(Succeed())
						volumes := job.Spec.Template.Spec.Volumes
						pvc := &corev1.PersistentVolumeClaim{}
						pvc.Namespace = rs.Namespace
						found := false
						for _, v := range volumes {
							if v.PersistentVolumeClaim != nil {
								found = true
								pvc.Name = v.PersistentVolumeClaim.ClaimName
							}
						}
						Expect(found).To(BeTrue())
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
						Expect(pvc.Spec.DataSource.Name).To(Equal(srcPVC.Name))
						Expect(pvc).To(beOwnedBy(rs))
					})

					Context("Pausing an rclone sync job", func() {
						parallelism := int32(0)
						BeforeEach(func() {
							rs.Spec.Paused = true
						})
						It("job will be created but won't start", func() {
							Eventually(func() error {
								return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
							}, maxWait, interval).Should(Succeed())
							Expect(*job.Spec.Parallelism).To(Equal(parallelism))
						})
					})
				})
			})
			for i := range directCopyMethodTypes {
				Context(fmt.Sprintf("Rclone is provided schedule + CopyMethod%s, ", directCopyMethodTypes[i]), func() {
					directCopyMethodType := directCopyMethodTypes[i]
					var schedule string
					BeforeEach(func() {
						schedule = "4 * * * *"
						rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
							Schedule: &schedule,
						}
						// provide a copy method
						rs.Spec.Rclone.CopyMethod = directCopyMethodType
					})
					It("Uses the Source PVC as the sync source", func() {
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, maxWait, interval).Should(Succeed())
						// look for the source PVC in the job's volume list
						volumes := job.Spec.Template.Spec.Volumes
						found := false
						for _, v := range volumes {
							if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == srcPVC.Name {
								found = true
							}
						}
						Expect(found).To(BeTrue())
						Expect(srcPVC).NotTo(beOwnedBy(rs))
					})
				})
			}
		})
		When("rclone is given an incorrect config", func() {
			var emptyString = ""
			BeforeEach(func() {
				rs.Spec.Rclone = &volsyncv1alpha1.ReplicationSourceRcloneSpec{
					ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
						CopyMethod: volsyncv1alpha1.CopyMethodDirect,
					},
					RcloneConfig:        &emptyString,
					RcloneConfigSection: &emptyString,
					RcloneDestPath:      &emptyString,
				}
			})
			When("All fields are empty", func() {
				It("should not start", func() {
					Consistently(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, duration, interval).ShouldNot(Succeed())
				})
			})
			When("rclone has secret but nothing else", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
				})
				It("does not start", func() {
					Consistently(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, duration, interval).ShouldNot(Succeed())
				})
			})
			When("rclone has secret + rcloneConfigSection but not DestPath", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
					rs.Spec.Rclone.RcloneConfigSection = &configSection
				})
				It("Existing RcloneConfig + RcloneConfigSection", func() {
					Consistently(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, duration, interval).ShouldNot(Succeed())
				})
			})
			When("rclone has all fields filled", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.RcloneConfig = &rcloneSecret.Name
					rs.Spec.Rclone.RcloneConfigSection = &configSection
					rs.Spec.Rclone.RcloneDestPath = &destPath
				})
				It("should start", func() {
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, maxWait, interval).Should(Succeed())
				})
			})
		})
	})
})
