package utils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Utils", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	var testNamespace *corev1.Namespace
	var rd *volsyncv1alpha1.ReplicationDestination
	var pvc *corev1.PersistentVolumeClaim

	BeforeEach(func() {
		// Create namespace for test
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ns-utilstests-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())
		Expect(testNamespace.Name).NotTo(BeEmpty())

		// Create a replication destination
		rd = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rd-utils-test-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				External: &volsyncv1alpha1.ReplicationDestinationExternalSpec{},
			},
		}
		Expect(k8sClient.Create(ctx, rd)).To(Succeed())

		// Create a PVC as well
		capacity := resource.MustParse("1Gi")
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "pvc-util-test-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: capacity,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
	})

	Describe("OwnerRef and Labels on objects", func() {
		It("Should add ownerRef and volsync label(s) to an object", func() {
			// Testing using a PVC as the object
			Expect(utils.AddControllerReferenceAndVolSyncLabels(rd, pvc, k8sClient.Scheme(), logger)).To(Succeed())

			foundOwner := false
			for _, ownerRef := range pvc.GetOwnerReferences() {
				if ownerRef.Name == rd.GetName() && ownerRef.Kind == "ReplicationDestination" && ownerRef.UID == rd.GetUID() {
					// confirm ownerref should indicate it's a controller
					Expect(ownerRef.Controller).NotTo(BeNil())
					Expect(*ownerRef.Controller).To(BeTrue())
					foundOwner = true
				}
			}
			Expect(foundOwner).To(BeTrue())

			labelVal, ok := pvc.GetLabels()[utils.VolsyncCreatedByLabelKey]
			Expect(ok).To(BeTrue())
			Expect(labelVal).To(Equal(utils.VolsyncCreatedByLabelValue))
		})
	})
})
