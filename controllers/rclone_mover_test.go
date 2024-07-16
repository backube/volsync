package controllers

import (
	"fmt"
	"strings"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const rCloneDstJobPrefix = "volsync-rclone-dst-"

//nolint:dupl
var _ = Describe("ReplicationDestination [rclone]", func() {
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
		createWithCacheReload(ctx, k8sClient, namespace)
		Expect(namespace.Name).NotTo(BeEmpty())

		// sets up RD, Rclone, Secret & PVC spec
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
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
				Name:      rCloneDstJobPrefix + rd.Name,
				Namespace: rd.Namespace,
			},
		}
	})
	AfterEach(func() {
		// delete each namespace on shutdown so resources can be reclaimed
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// create necessary resources
		createWithCacheReload(ctx, k8sClient, pvc)
		createWithCacheReload(ctx, k8sClient, rcloneSecret)
		createWithCacheReload(ctx, k8sClient, rd)
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
							Eventually(func() bool {
								inst := &volsyncv1alpha1.ReplicationDestination{}
								if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst); err != nil {
									return false
								}
								return inst.Status != nil && inst.Status.NextSyncTime != nil
							}, maxWait, interval).Should(BeTrue())
							//Expect(inst.Status.NextSyncTime).NotTo(BeNil())
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
								err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
								if err != nil || inst == nil || inst.Status == nil {
									return nil
								}
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
										Name:      rCloneDstJobPrefix + rd.Name,
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

					It("Ensure that previous VolumeSnapshot is deleted at the end of an iteration", func() {
						// get list of volume snapshots in the ns - this is the 1st snapshot
						snapshots := &snapv1.VolumeSnapshotList{}
						Eventually(func() []snapv1.VolumeSnapshot {
							_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
							return snapshots.Items
						}, maxWait, interval).Should(Not(BeEmpty()))
						Expect(len(snapshots.Items)).To(Equal(1))

						// sync should be waiting for snapshot - check that lastSyncStartTime
						// is set
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).To(Succeed())
						Expect(rd.Status.LastSyncStartTime).Should(Not(BeNil()))

						// update the VS name
						snapshot1 := snapshots.Items[0]
						foo := "dummysnapshot"
						snapshot1.Status = &snapv1.VolumeSnapshotStatus{
							BoundVolumeSnapshotContentName: &foo,
						}
						Expect(k8sClient.Status().Update(ctx, &snapshot1)).To(Succeed())
						// wait for an image to be set for RD
						Eventually(func() *corev1.TypedLocalObjectReference {
							_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							return rd.Status.LatestImage
						}, maxWait, interval).Should(Not(BeNil()))
						latestImage1 := rd.Status.LatestImage
						// ensure the name was correctly set
						Expect(latestImage1.Kind).To(Equal("VolumeSnapshot"))
						Expect(*latestImage1.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
						Expect(latestImage1.Name).To(Equal(snapshot1.GetName()))
						// Ensure the duration was set
						Expect(rd.Status.LastSyncDuration).Should(Not(BeNil()))
						// Ensure the lastSyncStartTime was unset
						Expect(rd.Status.LastSyncStartTime).Should(BeNil())

						// Sync completed, Job should now get cleaned up
						Eventually(func() bool {
							jobFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
							return kerrors.IsNotFound(jobFoundErr)
						}, maxWait, interval).Should(BeTrue())

						Eventually(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							if err != nil {
								return false
							}
							synchronizingCondition := apimeta.FindStatusCondition(rd.Status.Conditions,
								volsyncv1alpha1.ConditionSynchronizing)
							if synchronizingCondition == nil {
								return false
							}
							return (synchronizingCondition.Status == metav1.ConditionFalse &&
								synchronizingCondition.Reason == volsyncv1alpha1.SynchronizingReasonSched)
						}, maxWait, interval).Should(BeTrue())

						// About to trigger another sync - Snapshots use a time format for naming that uses seconds
						// make sure test isn't running so fast that the next sync could use the same snapshot name
						now := time.Now().Format("20060102150405")
						snap1Name := snapshot1.GetName()
						snap1NameSplit := strings.Split(snap1Name, "-")
						snap1Time := snap1NameSplit[len(snap1NameSplit)-1]
						if snap1Time == now {
							// Sleep to make sure next snapshot will not have the same name as previous
							time.Sleep(1 * time.Second)
						}

						//
						// Now manually trigger another sync to generate another snapshot
						//
						manualTrigger := "testrightnow1"
						Eventually(func() error {
							// Put this in Eventually loop to avoid update issues (controller is also updating the rd)
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							if err != nil {
								return err
							}
							// Update RD with manual trigger to force another sync
							rd.Spec.Trigger.Manual = manualTrigger
							return k8sClient.Update(ctx, rd)
						}, maxWait, interval).Should(Succeed())

						// Job should be recreated for 2nd sync, force 2nd job to succeed
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
						}, maxWait, interval).Should(Succeed())

						// sync should be waiting for job to complete - before forcing job to succeed,
						// check that lastSyncStartTime is set
						Eventually(func() *metav1.Time {
							if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd); err != nil {
								return nil
							}
							return rd.Status.LastSyncStartTime
						}, maxWait, interval).ShouldNot(BeNil())

						job.Status.Succeeded = 1
						Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

						snapshotsAfter2ndSync := &snapv1.VolumeSnapshotList{}
						Eventually(func() []snapv1.VolumeSnapshot {
							_ = k8sClient.List(ctx, snapshotsAfter2ndSync, client.InNamespace(rd.Namespace))
							return snapshotsAfter2ndSync.Items
						}, maxWait, interval).Should(HaveLen(2))
						// Find the new VS and update its BoundVolumeSnapshotContentName
						var snapshot2 snapv1.VolumeSnapshot
						for _, sn := range snapshotsAfter2ndSync.Items {
							if sn.GetName() != snapshot1.GetName() {
								snapshot2 = sn
								break
							}
						}
						Expect(snapshot2.GetName).To(Not(Equal("")))
						foo2 := "dummysnapshot2"
						snapshot2.Status = &snapv1.VolumeSnapshotStatus{
							BoundVolumeSnapshotContentName: &foo2,
						}
						Expect(k8sClient.Status().Update(ctx, &snapshot2)).To(Succeed())

						// wait for an image to be set for RD
						Eventually(func() *corev1.TypedLocalObjectReference {
							_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							if rd.Status.LatestImage == nil || rd.Status.LatestImage.Name == snapshot1.GetName() {
								// Return nil if the status is still reporting the previous image
								return nil
							}
							return rd.Status.LatestImage
						}, maxWait, interval).Should(Not(BeNil()))
						latestImage2 := rd.Status.LatestImage
						// ensure the name was correctly set
						Expect(latestImage2.Kind).To(Equal("VolumeSnapshot"))
						Expect(*latestImage2.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
						Expect(latestImage2.Name).To(Equal(snapshot2.GetName()))
						Expect(rd.Status.LastManualSync).To(Equal(manualTrigger))

						// Sync 2 completed, job and previous snapshot should be cleaned up
						Eventually(func() bool {
							jobFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
							return kerrors.IsNotFound(jobFoundErr)
						}, maxWait, interval).Should(BeTrue())

						Eventually(func() bool {
							snapFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot1), &snapshot1)
							return kerrors.IsNotFound(snapFoundErr)
						}, maxWait, interval).Should(BeTrue())

						// Confirm the latest snapshot is not cleaned up
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot2), &snapshot2)).To(Succeed())

						// Re-load RD to ensure we have the latest status after sync cycle completion
						// and then check status conditions
						Eventually(func() *metav1.Condition {
							_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							reconcileCondition := apimeta.FindStatusCondition(rd.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
							if reconcileCondition == nil {
								return nil
							}
							// Should be waiting for next manual sync
							if reconcileCondition.Status != metav1.ConditionFalse ||
								reconcileCondition.Reason != volsyncv1alpha1.SynchronizingReasonManual {
								return nil
							}
							return reconcileCondition
						}, maxWait, interval).Should(Not(BeNil()))

						// Ensure the duration was set
						Expect(rd.Status.LastSyncDuration).Should(Not(BeNil()))
						// Ensure the lastSyncStartTime was unset
						Expect(rd.Status.LastSyncStartTime).Should(BeNil())

						Eventually(func() *metav1.Condition {
							_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
							syncCondition := apimeta.FindStatusCondition(rd.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
							if syncCondition == nil {
								return nil
							}
							if syncCondition.Status != metav1.ConditionFalse {
								return nil
							}
							if syncCondition.Reason != volsyncv1alpha1.SynchronizingReasonManual {
								return nil
							}
							return syncCondition
						}, maxWait, interval).Should(Not(BeNil()))
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
				Eventually(func() bool {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
					if inst.Status == nil {
						return false
					}
					reconcileCondition := apimeta.FindStatusCondition(inst.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
					if reconcileCondition == nil {
						return false
					}
					if reconcileCondition.Status != metav1.ConditionFalse ||
						reconcileCondition.Reason != volsyncv1alpha1.SynchronizingReasonError {
						return false
					}
					return true
				}, duration, interval).Should(BeTrue())
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
						Name:      rCloneDstJobPrefix + rd.Name,
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
		createWithCacheReload(ctx, k8sClient, namespace)
		Expect(namespace.Name).NotTo(BeEmpty())
		srcPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thesource",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
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
	})
	AfterEach(func() {
		// delete each namespace on shutdown so resources can be reclaimed
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// at least the srcPVC & rcloneSecret should be expected to start at this point
		createWithCacheReload(ctx, k8sClient, srcPVC)
		createWithCacheReload(ctx, k8sClient, rcloneSecret)
	})
	When("Components are expected to start", func() {
		JustBeforeEach(func() {
			// source pvc comes up
			createWithCacheReload(ctx, k8sClient, rs)
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

			When("copyMethod of Snapshot is specified", func() {
				BeforeEach(func() {
					rs.Spec.Rclone.ReplicationSourceVolumeOptions.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot

					// Set a schedule
					var schedule = "0 0 1 1 *"
					rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
						Schedule: &schedule,
					}
				})

				var snapshot snapv1.VolumeSnapshot

				JustBeforeEach(func() {
					// Set snapshot to be bound so the source reconcile can proceed
					snapshots := &snapv1.VolumeSnapshotList{}
					Eventually(func() []snapv1.VolumeSnapshot {
						_ = k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))
						return snapshots.Items
					}, maxWait, interval).Should(Not(BeEmpty()))

					// update the VS name
					snapshot = snapshots.Items[0]
					foo := "dummysourcesnapshot"
					snapshot.Status = &snapv1.VolumeSnapshotStatus{
						BoundVolumeSnapshotContentName: &foo,
					}
					Expect(k8sClient.Status().Update(ctx, &snapshot)).To(Succeed())
				})

				It("creates a snapshot of the source PVC as the sync source", func() {
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
					Expect(pvc.Spec.DataSource.Name).To(Equal(snapshot.Name))
					Expect(pvc.Spec.DataSource.Kind).To(Equal("VolumeSnapshot"))
					Expect(pvc).To(beOwnedBy(rs))
				})

				It("Ensure that temp VolumeSnapshot and temp PVC are deleted at the end of an iteration", func() {
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

					// Force job status to succeeded
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, maxWait, interval).Should(Succeed())
					// just so the tests will run for now
					job.Status.Succeeded = 1
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

					// Check that temp pvc is cleaned up
					Eventually(func() bool {
						pvcFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
						if kerrors.IsNotFound(pvcFoundErr) {
							return true
						}
						// PVC may be stuck because of pvc finalizer in test scenario but check it's
						// marked for deletion
						return !pvc.GetDeletionTimestamp().IsZero()
					}, maxWait, interval).Should(BeTrue())

					// Check that temp snapshot is cleaned up
					Eventually(func() bool {
						snapFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot), &snapshot)
						return kerrors.IsNotFound(snapFoundErr)
					}, maxWait, interval).Should(BeTrue())
				})

				It("Ensure lastSyncDuration is set at the end of an iteration", func() {
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					}, maxWait, interval).Should(Succeed())
					// Job was found, so synchronization should be in-progress

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
					Expect(rs.Status.LastSyncStartTime).Should(Not(BeNil())) // Make sure start time was set

					// set the job to succeed so sync can finish
					job.Status.Succeeded = 1
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

					// Sync should complete and last sync duration should be set
					Eventually(func() bool {
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
						if rs.Status == nil || rs.Status.LastSyncDuration == nil {
							return false
						}
						return true
					}, maxWait, interval).Should(BeTrue())

					// Now confirm lastSyncStartTime was un-set
					// Note that LSST will become set again as soon as a new
					// iteration begins. This test's cronspec has been
					// configured to make it very unlikely for that to happen.
					Expect(rs.Status.LastSyncStartTime).Should(BeNil())
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
