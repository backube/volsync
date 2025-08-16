package cmd

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	krand "k8s.io/apimachinery/pkg/util/rand"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("migration delete", func() {
	var (
		ns                        *corev1.Namespace
		cmd                       *cobra.Command
		dirname                   string
		migrationRelationshipFile string
		migrationCmdArgs          = make(map[string]string)
	)

	BeforeEach(func() {
		// Defaults for tests
		migrationCmdArgs = map[string]string{
			"capacity": "2Gi",
			"pvcname":  "dest/volsync",
		}
	})

	JustBeforeEach(func() {
		ns = &corev1.Namespace{}
		cmd = &cobra.Command{}
		var mc *migrationCreate
		var err error

		initMigrationCreateCmd(cmd)
		cmd.Flags().String("relationship", "test", "")

		dirname, err = os.MkdirTemp("", "relation")
		Expect(err).NotTo(HaveOccurred())
		cmd.Flags().String("config-dir", dirname, "")

		err = migrationCmdArgsSet(cmd, migrationCmdArgs)
		Expect(err).ToNot(HaveOccurred())

		mc, err = newMigrationCreate(cmd)
		Expect(err).NotTo(HaveOccurred())

		mc.client = k8sClient
		mc.Namespace = "foo-" + krand.String(5)
		// create namespace
		ns, err = mc.ensureNamespace(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(ns).NotTo(BeNil())

		ns = &corev1.Namespace{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: mc.Namespace}, ns)).To(Succeed())

		PVC, err := mc.ensureDestPVC(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(PVC).NotTo(BeNil())

		mrd, err := mc.newMigrationRelationshipDestination()
		Expect(err).ToNot(HaveOccurred())
		Expect(mrd).ToNot(BeNil())
		mc.mr.data.Destination = mrd

		// Create replicationdestination
		rd, err := mc.mr.mh.EnsureReplicationDestination(context.Background(), mc.client, mrd)
		Expect(err).ToNot(HaveOccurred())
		Expect(rd).ToNot(BeNil())

		err = mc.mr.Save()
		Expect(err).ToNot(HaveOccurred())

		// Verify migration relationship file was created
		migrationRelationshipFile = dirname + "/test.yaml"
		_, err = os.Stat(migrationRelationshipFile)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if ns != nil {
			Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed())
		}

		os.RemoveAll(dirname)
	})

	It("verify migration delete methods with all probabilities", func() {
		mr, err := loadMigrationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())

		md := newMigrationDelete()
		md.mr = mr
		md.client = k8sClient

		err = md.deleteReplicationDestination(context.Background())
		Expect(err).To(BeNil())
		Expect(err).ToNot(HaveOccurred())

		err = md.mr.Delete()
		Expect(err).ToNot(HaveOccurred())

		_, err = os.Stat(migrationRelationshipFile)
		Expect(err).To(HaveOccurred())
		Expect(os.IsNotExist(err)).To(BeTrue())

		// Verify delete replicationdestination that does not exist
		err = md.deleteReplicationDestination(context.Background())
		Expect(err).To(HaveOccurred())

		// Verify deletion of relatioship file that does not exist
		err = md.mr.Delete()
		Expect(err).To(HaveOccurred())
	})

	Context("Loading older migrationship (data at v1)", func() {
		const v1MigrationRelationshipFileContents string = `data:
    destination:
        cluster: "test-cluster"
        destination:
            replicationdestinationvolumeoptions:
                accessmodes: []
                copymethod: ""
                destinationpvc: volsync
            servicetype: LoadBalancer
        namespace: dest
        pvcname: volsync
        rdname: dest-volsync-migration-dest
        sshkeyname: ""
    version: 1
id: c22818bd-8716-436f-a48d-c2c8746afde6
type: migration`

		When("The migration relationship file is at v1", func() {
			JustBeforeEach(func() {
				// Parent JustBeforeEach sets up a v2 migration relationship file - overwrite it
				// with a v1 file for this test
				Expect(os.WriteFile(migrationRelationshipFile, []byte(v1MigrationRelationshipFileContents), 0600)).To(Succeed())
			})

			It("Should convert data to v2", func() {
				mr, err := loadMigrationRelationship(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(mr.data.Version).To(Equal(2))
				Expect(mr.data.IsRsyncTLS).To(BeFalse())
				Expect(mr.data.Destination.RDName).To(Equal("dest-volsync-migration-dest"))
				Expect(mr.data.Destination.PVCName).To(Equal("volsync"))
				Expect(mr.data.Destination.Namespace).To(Equal("dest"))
				Expect(mr.data.Destination.Cluster).To(Equal("test-cluster"))
				Expect(*mr.data.Destination.ServiceType).To(Equal(corev1.ServiceTypeLoadBalancer))
				// Was not specified in v1 but should be filled out when we convert to v2
				Expect(mr.data.Destination.CopyMethod).To(Equal(volsyncv1alpha1.CopyMethodDirect))
			})
		})
	})

	Context("Test loading migrationrelationship file instantiates correctly", func() {
		Context("When the migrationRelationship uses rsync (the default)", func() {
			It("Should load correctly", func() {
				mr, err := loadMigrationRelationship(cmd)
				Expect(err).NotTo(HaveOccurred())

				Expect(mr.data.IsRsyncTLS).To(BeFalse())
				// Check the migrationHandler is Rsync
				_, ok := mr.mh.(*migrationHandlerRsync)
				Expect(ok).To(BeTrue())
			})
		})
		Context("When the migrationRelationship uses rsync-tls", func() {
			BeforeEach(func() {
				migrationCmdArgs["rsynctls"] = "true"
			})

			It("Should load correctly", func() {
				mr, err := loadMigrationRelationship(cmd)
				Expect(err).NotTo(HaveOccurred())

				Expect(mr.data.IsRsyncTLS).To(BeTrue())
				// Check the migrationHandler is RsyncTLS
				_, ok := mr.mh.(*migrationHandlerRsyncTLS)
				Expect(ok).To(BeTrue())
			})
		})
	})
})
