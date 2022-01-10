package cmd

import (

	//. "github.com/golang/mock/gomock"

	"context"
	"errors"
	"io/ioutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("migration delete", func() {
	var (
		mc               = &migrationCreate{}
		rd               = &volsyncv1alpha1.ReplicationDestination{}
		relationship     = &Relationship{}
		relationshipName = "v3"
		err              = errors.New("")
		dirname          = ""
	)
	When("relationship is deleted", func() {
		BeforeEach(func() {
			dirname, err = ioutil.TempDir("", "relation")
			Expect(err).NotTo(HaveOccurred())
			relationship, _ = createRelationship(dirname, relationshipName, MigrationRelationship)
			mc = &migrationCreate{
				clientObject: k8sClient,
				mr: &migrationRelationship{
					data: &migrationRelationshipData{
						Destination: &migrationRelationshipDestination{
							MDName:    "barfoo",
							Cluster:   "",
							Namespace: "testnamespace",
						},
					},
					Relationship: *relationship,
				},
			}
			pvcname := "testnamespace/barfoo"
			serviceType := "ClusterIP"
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "testnamespace",
				},
			}
			_ = mc.clientObject.Create(context.Background(), ns)

			rsyncSpec := &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					CopyMethod:     "Direct",
					DestinationPVC: &pvcname,
				},
				ServiceType: (*v1.ServiceType)(&serviceType),
			}
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "barfoo",
					Namespace: "testnamespace",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rsync: rsyncSpec,
				},
			}
		})

		It("verify delete destination relationship with reslationship and ReplicationDestination", func() {
			err = mc.clientObject.Create(context.Background(), rd)
			Expect(err).ToNot(HaveOccurred())

			err = mc.mr.Save()
			Expect(err).ToNot(HaveOccurred())

			err = mc.deleteReplicationDestination(context.Background())
			Expect(err).To(BeNil())

			Expect(err).ToNot(HaveOccurred())
			err = relationship.Delete()
			Expect(err).ToNot(HaveOccurred())

		})

		It("verify delete destination relationship without relationship", func() {
			err = mc.clientObject.Create(context.Background(), rd)
			Expect(err).ToNot(HaveOccurred())

			err = mc.deleteReplicationDestination(context.Background())
			Expect(err).To(BeNil())

			Expect(err).ToNot(HaveOccurred())
			err = relationship.Delete()
			Expect(err.Error()).Should(Equal("remove " + dirname + "/" + relationshipName +
				".yaml: no such file or directory"))

		})

		It("verify delete destination relationship without RelationshipDestination", func() {
			err = mc.mr.Save()
			Expect(err).ToNot(HaveOccurred())

			DeleteErr := mc.deleteReplicationDestination(context.Background())
			err = relationship.Delete()
			Expect(DeleteErr.Error()).To(Equal("migration destination not found"))
			Expect(err).To(BeNil())

		})
	})
})
