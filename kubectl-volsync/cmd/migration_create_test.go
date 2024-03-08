package cmd

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	krand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("migration", func() {
	var (
		ns               *corev1.Namespace
		cmd              *cobra.Command
		mc               *migrationCreate
		migrationCmdArgs = make(map[string]string)
		dirname          string
	)

	BeforeEach(func() {
		// Default values for tests
		migrationCmdArgs = map[string]string{
			"capacity": "2Gi",
			"pvcname":  "dest/volsync",
		}
	})

	JustBeforeEach(func() {
		ns = &corev1.Namespace{}
		cmd = &cobra.Command{}
		var err error

		initMigrationCreateCmd(cmd)
		cmd.Flags().String("relationship", "test", "")

		dirname, err = os.MkdirTemp("", "relation")
		Expect(err).NotTo(HaveOccurred())
		cmd.Flags().String("config-dir", dirname, "")

		err = migrationCmdArgsSet(cmd, migrationCmdArgs)
		Expect(err).ToNot(HaveOccurred())

		mc, err = newMigrationCreate(cmd)
		Expect(err).ToNot(HaveOccurred())

		mc.client = k8sClient
		mc.Namespace = "foo-" + krand.String(5)
		// create namespace
		ns, err = mc.ensureNamespace(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(ns).NotTo(BeNil())

		ns = &corev1.Namespace{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: mc.Namespace}, ns)).To(Succeed())

	})

	AfterEach(func() {
		if ns != nil {
			Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed())
		}

		os.RemoveAll(dirname)
	})

	It("Verify migration create arguments: fails with missing pvcname arg, with no pre-existing PVC", func() {
		err := cmd.Flags().Set("pvcname", "")
		Expect(err).NotTo(HaveOccurred())
		err = mc.parseCLI(cmd)
		Expect(err).To(HaveOccurred())
	})

	It("Verify migration create arguments: works with minimum set of arguments, with no preexsting PVC ", func() {
		err := mc.parseCLI(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Verify migration create arguments: fails with wrong accessMode arg value", func() {
		err := cmd.Flags().Set("accessmodes", "Read-Write-Once")
		Expect(err).NotTo(HaveOccurred())
		err = mc.parseCLI(cmd)
		Expect(err).To(HaveOccurred())
	})

	It("Verify migration create arguments: fails with wrong capacity arg value", func() {
		err := cmd.Flags().Set("capacity", "2GB")
		Expect(err).NotTo(HaveOccurred())
		err = mc.parseCLI(cmd)
		Expect(err).To(HaveOccurred())
	})

	Describe("Rsync-TLS moverSecurityContext flags", func() {
		moverSecCtxParams := map[string]string{
			"runasuser":          "5001",
			"runasgroup":         "201",
			"fsgroup":            "202",
			"runasnonroot":       "true",
			"seccompprofiletype": "RuntimeDefault",
		}

		Context("When rsync-tls is not used", func() {
			It("Should ignore the moverSecurityContext flags", func() {
				Expect(mc.IsRsyncTLS).To(BeFalse())

				for k, v := range moverSecCtxParams {
					Expect(cmd.Flags().Set(k, v)).To(Succeed())
					Expect(mc.parseCLI(cmd)).To(Succeed())

					// Mover Security context should not be set (params ignored when rsynctls is not used)
					Expect(mc.MoverSecurityContext).To(BeNil())
				}
			})
		})
		Context("When rsync-tls is not used", func() {
			BeforeEach(func() {
				migrationCmdArgs["rsynctls"] = "True"
			})

			Context("Parsing flags when they are set individually", func() {
				for k := range moverSecCtxParams {
					Context(fmt.Sprintf("When only the %s moverSecurityContext flag is set", k), func() {
						flagName := k
						flagValue := moverSecCtxParams[k]
						It("Should parse the flag correctly", func() {
							Expect(mc.IsRsyncTLS).To(BeTrue())
							Expect(cmd.Flags().Set(flagName, flagValue)).To(Succeed())
							Expect(mc.parseCLI(cmd)).To(Succeed())
							Expect(mc.MoverSecurityContext).NotTo(BeNil())
						})
					})
				}
			})

			Context("When using moverSecurityContext flags", func() {
				BeforeEach(func() {
					for k, v := range moverSecCtxParams {
						migrationCmdArgs[k] = v
					}
				})
				It("Should configure the moverSecurityContext correctly", func() {
					Expect(mc.IsRsyncTLS).To(BeTrue())
					Expect(mc.MoverSecurityContext).NotTo(BeNil())
					Expect(mc.MoverSecurityContext).To(Equal(&corev1.PodSecurityContext{
						RunAsUser:    ptr.To[int64](5001),
						RunAsGroup:   ptr.To[int64](201),
						FSGroup:      ptr.To[int64](202),
						RunAsNonRoot: ptr.To[bool](true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					}))
				})
			})
		})
	})

	It("Ensure namespace creation", func() {
		ns = &corev1.Namespace{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: mc.Namespace}, ns)).To(Succeed())

		// call ensureNamespace(), should return namespace.
		ns, err := mc.ensureNamespace(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(ns).NotTo(BeNil())

		ns = &corev1.Namespace{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: mc.Namespace}, ns)).To(Succeed())

	})

	It("Ensure PVC creation", func() {
		// Create destination PVC
		PVC, err := mc.ensureDestPVC(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(PVC).NotTo(BeNil())

		PVC = &corev1.PersistentVolumeClaim{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
			Name: mc.DestinationPVC}, PVC)).To(Succeed())

		// Retry creating a PVC, should return existing PVC
		PVC, err = mc.ensureDestPVC(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(PVC).NotTo(BeNil())

		PVC = &corev1.PersistentVolumeClaim{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
			Name: mc.DestinationPVC}, PVC)).To(Succeed())

	})

	Context("When using rsync (the default)", func() {
		//nolint:dupl
		It("Ensure replicationdestination creation", func() {
			// Create a PVC
			PVC, err := mc.ensureDestPVC(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(PVC).NotTo(BeNil())
			PVC = &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.DestinationPVC}, PVC)).To(Succeed())

			mrd, err := mc.newMigrationRelationshipDestination()
			Expect(err).ToNot(HaveOccurred())
			Expect(mrd).ToNot(BeNil())
			mc.mr.data.Destination = mrd

			// data version should be 2
			Expect(mc.mr.data.Version).To(Equal(2))

			// Should use rsync SSH by default
			Expect(mc.mr.data.IsRsyncTLS).To(BeFalse())
			// Check the migrationHandler is RsyncTLS
			_, ok := mc.mr.mh.(*migrationHandlerRsync)
			Expect(ok).To(BeTrue())

			// Create replicationdestination
			rd, err := mc.mr.mh.EnsureReplicationDestination(context.Background(), mc.client, mc.mr.data.Destination)
			Expect(err).ToNot(HaveOccurred())
			Expect(rd).ToNot(BeNil())
			rd = &volsyncv1alpha1.ReplicationDestination{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.RDName}, rd)).To(Succeed())

			Expect(rd.Spec.Rsync).NotTo(BeNil()) // Rsync Spec should be used
			Expect(rd.Spec.RsyncTLS).To(BeNil())

			// Post status field in rd to mock controller
			address := "Volsync-mock-address"
			sshKey := "Volsync-mock-sshKey"
			rd.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
				Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncStatus{
					Address: &address,
					SSHKeys: &sshKey,
				}}
			Expect(k8sClient.Status().Update(context.Background(), rd)).To(Succeed())
			// Wait for mock address and sshKey to pop up
			_, err = mc.mr.mh.WaitForRDStatus(context.Background(), mc.client, rd)
			Expect(err).ToNot(HaveOccurred())
			// Retry creation of replicationdestination and it should fail as destination already exists
			rd, err = mc.mr.mh.EnsureReplicationDestination(context.Background(), mc.client, mc.mr.data.Destination)
			Expect(err).To(HaveOccurred())
			Expect(rd).To(BeNil())
			rd = &volsyncv1alpha1.ReplicationDestination{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.RDName}, rd)).To(Succeed())
			// Should return existing address and sshkey
			_, err = mc.mr.mh.WaitForRDStatus(context.Background(), mc.client, rd)
			Expect(err).ToNot(HaveOccurred())
		})

	})
	Context("When using rsync-tls", func() {
		BeforeEach(func() {
			migrationCmdArgs["rsynctls"] = "true"
		})
		//nolint:dupl
		It("Ensure replicationdestination (rsynctls) creation", func() {
			// Create a PVC
			PVC, err := mc.ensureDestPVC(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(PVC).NotTo(BeNil())
			PVC = &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.DestinationPVC}, PVC)).To(Succeed())

			mrd, err := mc.newMigrationRelationshipDestination()
			Expect(err).ToNot(HaveOccurred())
			Expect(mrd).ToNot(BeNil())
			mc.mr.data.Destination = mrd

			// data version should be 2
			Expect(mc.mr.data.Version).To(Equal(2))

			// Should use rsync TLS by default
			Expect(mc.mr.data.IsRsyncTLS).To(BeTrue())
			// Check the migrationHandler is RsyncTLS
			_, ok := mc.mr.mh.(*migrationHandlerRsyncTLS)
			Expect(ok).To(BeTrue())

			// Create replicationdestination
			rd, err := mc.mr.mh.EnsureReplicationDestination(context.Background(), mc.client, mc.mr.data.Destination)
			Expect(err).ToNot(HaveOccurred())
			Expect(rd).ToNot(BeNil())
			rd = &volsyncv1alpha1.ReplicationDestination{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.RDName}, rd)).To(Succeed())

			Expect(rd.Spec.RsyncTLS).NotTo(BeNil()) // RsyncTLS Spec should be used
			Expect(rd.Spec.Rsync).To(BeNil())

			// Post status field in rd to mock controller
			address := "Volsync-mock-address"
			keys := "Volsync-mock-ks"
			rd.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
				RsyncTLS: &volsyncv1alpha1.ReplicationDestinationRsyncTLSStatus{
					Address:   &address,
					KeySecret: &keys,
				}}
			Expect(k8sClient.Status().Update(context.Background(), rd)).To(Succeed())
			// Wait for mock address and keySecret to pop up
			_, err = mc.mr.mh.WaitForRDStatus(context.Background(), mc.client, rd)
			Expect(err).ToNot(HaveOccurred())
			// Retry creation of replicationdestination and it should fail as destination already exists
			rd, err = mc.mr.mh.EnsureReplicationDestination(context.Background(), mc.client, mc.mr.data.Destination)
			Expect(err).To(HaveOccurred())
			Expect(rd).To(BeNil())
			rd = &volsyncv1alpha1.ReplicationDestination{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
				Name: mc.RDName}, rd)).To(Succeed())
			// Should return existing address and keysec
			_, err = mc.mr.mh.WaitForRDStatus(context.Background(), mc.client, rd)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

func migrationCmdArgsSet(cmd *cobra.Command, migrationCmdArgs map[string]string) error {
	for i, v := range migrationCmdArgs {
		err := cmd.Flags().Set(i, v)
		if err != nil {
			return err
		}
	}

	return nil
}
