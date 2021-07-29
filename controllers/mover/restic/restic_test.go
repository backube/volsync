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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
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

var _ = Describe("Restic prune policy", func() {
	var m *Mover
	var owner *v1.ConfigMap
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var start metav1.Time

	BeforeEach(func() {
		start = metav1.Now()
		// The underlying type of owner doesn't matter
		owner = &v1.ConfigMap{
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
			Register()
		})
		It("is added to the mover catalog", func() {
			found := false
			for _, v := range mover.Catalog {
				if v.(*Builder) != nil {
					found = true
				}
			}
			Expect(found).To(BeTrue())
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
			builder := Builder{}
			m, e := builder.FromSource(k8sClient, logger, rs)
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
			builder := Builder{}
			m, e := builder.FromDestination(k8sClient, logger, rd)
			Expect(m).To(BeNil())
			Expect(e).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Restic as a source", func() {
	var ctx = context.TODO()
	var ns *v1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rs *volsyncv1alpha1.ReplicationSource
	var sPVC *v1.PersistentVolumeClaim
	var mover *Mover
	BeforeEach(func() {
		// Create namespace for test
		ns = &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())

		sc := "spvcsc"
		sPVC = &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s",
				Namespace: ns.Name,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{
					v1.ReadWriteOnce,
				},
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
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
			b := Builder{}
			var err error
			m, err := b.FromSource(k8sClient, logger, rs)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})

		Context("validate repo secret", func() {
			var repo *v1.Secret
			BeforeEach(func() {
				repo = &v1.Secret{
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
					Eventually(func() bool {
						s, e := mover.validateRepository(ctx)
						if td.ok {
							return s != nil && e == nil
						}
						return s == nil && e != nil
					}, "5s", "1s").Should(BeTrue())
				}
			})
		})

		Context("Restic cache is created correctly", func() {
			var dataPVC *v1.PersistentVolumeClaim
			BeforeEach(func() {
				dataPVC = sPVC
			})

			When("no capacity is specified", func() {
				BeforeEach(func() {
					rs.Spec.Restic.CacheCapacity = nil
				})
				It("is 1Gi is size", func() {
					oneGB := resource.MustParse("1Gi")
					cache, err := mover.ensureCache(ctx, dataPVC)
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
					cache, err := mover.ensureCache(ctx, dataPVC)
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
					cache, err := mover.ensureCache(ctx, dataPVC)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal(dataPVC.Spec.AccessModes))
				})
			})
			When("options accessMode is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteMany}
					rs.Spec.Restic.CacheAccessModes = nil
				})
				It("matches the specified option", func() {
					cache, err := mover.ensureCache(ctx, dataPVC)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany}))
				})
			})
			When("cache accessMode is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteMany}
					rs.Spec.Restic.CacheAccessModes = []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}
				})
				It("uses the cache-specific mode", func() {
					cache, err := mover.ensureCache(ctx, dataPVC)
					Expect(err).ToNot(HaveOccurred())
					Expect(cache.Spec.AccessModes).To(Equal([]v1.PersistentVolumeAccessMode{v1.ReadOnlyMany}))
				})
			})

			When("no storageClass is set", func() {
				BeforeEach(func() {
					rs.Spec.Restic.StorageClassName = nil
					rs.Spec.Restic.CacheStorageClassName = nil
				})
				It("uses the cluster default", func() {
					cache, err := mover.ensureCache(ctx, dataPVC)
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
					cache, err := mover.ensureCache(ctx, dataPVC)
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
					cache, err := mover.ensureCache(ctx, dataPVC)
					Expect(err).ToNot(HaveOccurred())
					Expect(*cache.Spec.StorageClassName).To(Equal(cachesc))
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
			})
		})

		Context("mover Job is handled properly", func() {
			var jobName string
			var cache *v1.PersistentVolumeClaim
			var sa *v1.ServiceAccount
			var repo *v1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-src-" + rs.Name
				cache = &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thecache",
						Namespace: ns.Name,
					},
				}
				sPVC.Spec.DeepCopyInto(&cache.Spec)
				sa = &v1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				repo = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysecret",
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				resticContainerImage = "thecontainerimage"
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have only the backup action", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("backup"))
				})
				It("should use the specified container image", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Image
					Expect(args).To(Equal(resticContainerImage))
				})
				It("should use the specified service account", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
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
				It("should support pausing", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
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
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						err := k8sClient.Get(ctx, nsn, job)
						Expect(err).ToNot(HaveOccurred())
						return *job.Spec.Parallelism
					}).Should(Equal(int32(0)))

					mover.paused = false
					Eventually(func() int32 {
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo)
						Expect(e).NotTo(HaveOccurred())
						Expect(j).To(BeNil()) // hasn't completed
						err := k8sClient.Get(ctx, nsn, job)
						Expect(err).ToNot(HaveOccurred())
						return *job.Spec.Parallelism
					}).Should(Equal(int32(1)))

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
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}, timeout, interval).Should(Succeed())
					Expect(mover.shouldPrune(time.Now())).To(BeTrue())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("backup", "prune"))
					// Mark completed
					job.Status.Succeeded = int32(1)
					Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
					Eventually(func() bool {
						j, e = mover.ensureJob(ctx, cache, sPVC, sa, repo)
						return j != nil && e == nil
					}, timeout, interval).Should(BeTrue())
					Expect(mover.sourceStatus.LastPruned.Time.After(lastMonth.Time))
				})
			})
			When("the job has failed", func() {
				It("should be restarted", func() {
					j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
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
						j, e := mover.ensureJob(ctx, cache, sPVC, sa, repo)
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

var _ = Describe("Restic as a destination", func() {
	var ctx = context.TODO()
	var ns *v1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var rd *volsyncv1alpha1.ReplicationDestination
	var mover *Mover
	BeforeEach(func() {
		// Create namespace for test
		ns = &v1.Namespace{
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
			b := Builder{}
			var err error
			m, err := b.FromDestination(k8sClient, logger, rd)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).NotTo(BeNil())
			mover, _ = m.(*Mover)
			Expect(mover).NotTo(BeNil())
		})
		When("no destination volume is supplied", func() {
			var cap resource.Quantity
			var am v1.PersistentVolumeAccessMode
			BeforeEach(func() {
				am = v1.ReadWriteMany
				rd.Spec.Restic.AccessModes = []v1.PersistentVolumeAccessMode{
					am,
				}
				cap = resource.MustParse("6Gi")
				rd.Spec.Restic.Capacity = &cap
			})
			It("creates a temporary PVC", func() {
				pvc, e := mover.ensureDestinationPVC(ctx)
				Expect(e).NotTo(HaveOccurred())
				Expect(pvc).NotTo(BeNil())
				Expect(pvc.Spec.AccessModes).To(ConsistOf(am))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(cap))
			})
		})
		When("a destination volume is supplied", func() {
			var dPVC *v1.PersistentVolumeClaim
			BeforeEach(func() {
				dPVC = &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dest",
						Namespace: ns.Name,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{
							v1.ReadWriteOnce,
						},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
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
		})
		When("the service account is created", func() {
			It("exists", func() {
				sa, err := mover.ensureSA(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(sa).NotTo(BeNil())
				saName := sa.Name
				sa2 := &v1.ServiceAccount{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      saName,
						Namespace: ns.Name,
					}, sa2)
				}, timeout, interval).Should(Succeed())
				Expect(sa2.Name).To(Equal(sa.Name))
			})
		})
		Context("mover Job is handled properly", func() {
			var jobName string
			var dPVC *v1.PersistentVolumeClaim
			var cache *v1.PersistentVolumeClaim
			var sa *v1.ServiceAccount
			var repo *v1.Secret
			var job *batchv1.Job
			BeforeEach(func() {
				// hardcoded since we don't get access unless the job is
				// completed
				jobName = "volsync-dst-" + rd.Name
				dPVC = &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dest",
						Namespace: ns.Name,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{
							v1.ReadWriteOnce,
						},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				cache = &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thecache",
						Namespace: ns.Name,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{
							v1.ReadWriteOnce,
						},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}
				sa = &v1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "thesa",
						Namespace: ns.Name,
					},
				}
				repo = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mysecret",
						Namespace: ns.Name,
					},
				}
			})
			JustBeforeEach(func() {
				resticContainerImage = "thecontainerimage"
				Expect(k8sClient.Create(ctx, dPVC)).To(Succeed())
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())
				Expect(k8sClient.Create(ctx, sa)).To(Succeed())
				Expect(k8sClient.Create(ctx, repo)).To(Succeed())
			})
			When("it's the initial sync", func() {
				It("should have only the restore action", func() {
					j, e := mover.ensureJob(ctx, cache, dPVC, sa, repo)
					Expect(e).NotTo(HaveOccurred())
					Expect(j).To(BeNil()) // hasn't completed
					nsn := types.NamespacedName{Name: jobName, Namespace: ns.Name}
					job = &batchv1.Job{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, nsn, job)
						return err
					}).Should(Succeed())
					Expect(len(job.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
					args := job.Spec.Template.Spec.Containers[0].Args
					Expect(args).To(ConsistOf("restore"))
				})
			})
		})
	})
})
