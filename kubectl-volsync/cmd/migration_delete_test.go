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
)

var _ = Describe("migration delete", func() {
	var (
		ns      *corev1.Namespace
		cmd     *cobra.Command
		dirname string
	)

	BeforeEach(func() {
		ns = &corev1.Namespace{}
		cmd = &cobra.Command{}
		mc := &migrationCreate{}
		var err error

		migrationCmdArgs := map[string]string{
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
		rd, err := mc.ensureReplicationDestination(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(rd).ToNot(BeNil())

		err = mc.mr.Save()
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

		// Verify delete replicationdestination that does not exist
		err = md.deleteReplicationDestination(context.Background())
		Expect(err).To(HaveOccurred())

		// Verify deletion of relatioship file that does not exist
		err = md.mr.Delete()
		Expect(err).To(HaveOccurred())
	})

})
