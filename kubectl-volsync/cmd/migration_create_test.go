package cmd

import (
	"context"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	krand "k8s.io/apimachinery/pkg/util/rand"

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
		ns = &corev1.Namespace{}
		cmd = &cobra.Command{}
		mc = &migrationCreate{}
		var err error

		migrationCmdArgs = map[string]string{
			"capacity": "2Gi",
			"pvcname":  "dest/volsync",
		}

		initMigrationCreateCmd(cmd)
		cmd.Flags().String("relationship", "test", "")

		dirname, err = ioutil.TempDir("", "relation")
		Expect(err).NotTo(HaveOccurred())
		cmd.Flags().String("config-dir", dirname, "")

		mr, err := newMigrationRelationship(cmd)
		Expect(err).ToNot(HaveOccurred())
		Expect(mr).ToNot(BeNil())
		mc.mr = mr

		err = migrationCmdArgsSet(cmd, migrationCmdArgs)
		Expect(err).ToNot(HaveOccurred())

		err = mc.parseCLI(cmd)
		Expect(err).NotTo(HaveOccurred())

		mc.clientObject = k8sClient
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

		// Create replicationdestination
		rd, err := mc.ensureReplicationDestination(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(rd).ToNot(BeNil())
		rd = &volsyncv1alpha1.ReplicationDestination{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
			Name: mc.RDName}, rd)).To(Succeed())

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
		err = mc.waitForRDStatus(context.Background())
		Expect(err).ToNot(HaveOccurred())
		// Retry creation of replicationdestination and it should fail as destination already exists
		rd, err = mc.ensureReplicationDestination(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(rd).To(BeNil())
		rd = &volsyncv1alpha1.ReplicationDestination{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Namespace: mc.Namespace,
			Name: mc.RDName}, rd)).To(Succeed())
		// Should return existing address and sshkey
		err = mc.waitForRDStatus(context.Background())
		Expect(err).ToNot(HaveOccurred())
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
