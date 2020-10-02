package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

const (
	duration = 5 * time.Second
	maxWait  = 30 * time.Second
	interval = 250 * time.Millisecond
)

// beOwnedBy is a GomegaMatcher that ensures a Kubernetes Object is owned by a
// specific other object.
func beOwnedBy(expected interface{}) gomegatypes.GomegaMatcher {
	return &ownerRefMatcher{
		expected: expected,
	}
}

type ownerRefMatcher struct {
	expected interface{}
	reason   string
}

func (m *ownerRefMatcher) Match(actual interface{}) (success bool, err error) {
	actObj, ok := actual.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("actual value is not a metav1.Object")
	}
	expObj, ok := m.expected.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("expected value is not a metav1.Object")
	}
	controller := metav1.GetControllerOf(actObj)
	if controller == nil {
		m.reason = "it does not have an owner"
		return false, nil
	}
	if controller.UID != expObj.GetUID() {
		m.reason = "it does not refer to the expected parent object"
		return false, nil
	}
	return true, nil
}
func (m *ownerRefMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto be owned by\n\t%#v\nbut %v", actual, m.expected, m.reason)
}
func (m *ownerRefMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to be owned by\n\t%#v", actual, m.expected)
}

var _ = Describe("ReplicationDestination", func() {
	var rd *scribev1alpha1.ReplicationDestination
	var rdNsN types.NamespacedName
	var namespace *corev1.Namespace
	var ctx = context.Background()

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "scribe-test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())
		rd = &scribev1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
		}
		rdNsN = types.NamespacedName{Name: rd.Name, Namespace: rd.Namespace}
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, rd)).To(Succeed())
		// Wait for it to show up in the API server
		Eventually(func() error {
			inst := &scribev1alpha1.ReplicationDestination{}
			return k8sClient.Get(ctx, rdNsN, inst)
		}, maxWait, interval).Should(Succeed())
	})
	Context("when an unknown replication method is specified", func() {
		BeforeEach(func() {
			rd.Spec.ReplicationMethod = "somethingUnknown"
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *scribev1alpha1.ReplicationDestinationStatus {
				Expect(k8sClient.Get(ctx, rdNsN, rd)).To(Succeed())
				return rd.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when using rsync replication", func() {
		BeforeEach(func() {
			rd.Spec.ReplicationMethod = scribev1alpha1.ReplicationMethodRsync
		})

		Context("verify common Service settings", func() {
			svc := &corev1.Service{}
			JustBeforeEach(func() {
				Eventually(func() error {
					svcName := types.NamespacedName{
						Name:      "scribe-rsync-dest-" + rd.Name,
						Namespace: rd.Namespace,
					}
					return k8sClient.Get(ctx, svcName, svc)
				}, maxWait, interval).Should(Succeed())
			})
			It("Type defaults to ClusterIP", func() {
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(svc.Spec.ClusterIP).To(MatchRegexp(`^(\d+.){3}\d+$`))
			})
			It("opens a single port for ssh", func() {
				Expect(svc.Spec.Ports).To(HaveLen(1))
				thePort := svc.Spec.Ports[0]
				Expect(thePort.Name).To(Equal("ssh"))
				Expect(thePort.Port).To(Equal(int32(22)))
				Expect(thePort.Protocol).To(Equal(corev1.ProtocolTCP))
				Expect(thePort.TargetPort).To(Equal(intstr.FromInt(22)))
			})
			It("is owned by the ReplicationDestination", func() {
				Expect(svc).To(beOwnedBy(rd))
			})
		})

		Context("with serviceType of ClusterIP", func() {
			svc := &corev1.Service{}
			BeforeEach(func() {
				rd.Spec.Parameters = map[string]string{scribev1alpha1.RsyncServiceTypeKey: string(corev1.ServiceTypeClusterIP)}
			})
			It("a service of type ClusterIP is created", func() {
				Eventually(func() error {
					svcName := types.NamespacedName{
						Name:      "scribe-rsync-dest-" + rd.Name,
						Namespace: rd.Namespace,
					}
					return k8sClient.Get(ctx, svcName, svc)
				}, maxWait, interval).Should(Succeed())
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(svc.Spec.ClusterIP).To(MatchRegexp(`^(\d+.){3}\d+$`))
			})
		})

		Context("with serviceType of LoadBalancer", func() {
			svc := &corev1.Service{}
			BeforeEach(func() {
				rd.Spec.Parameters = map[string]string{scribev1alpha1.RsyncServiceTypeKey: string(corev1.ServiceTypeLoadBalancer)}
			})
			It("a service of type LoadBalancer is created", func() {
				Eventually(func() error {
					svcName := types.NamespacedName{
						Name:      "scribe-rsync-dest-" + rd.Name,
						Namespace: rd.Namespace,
					}
					return k8sClient.Get(ctx, svcName, svc)
				}, maxWait, interval).Should(Succeed())
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				Expect(svc.Spec.ClusterIP).To(MatchRegexp(`^(\d+.){3}\d+$`))
			})
		})

		Context("the ssh keys", func() {
			mainSecret := &corev1.Secret{}
			srcSecret := &corev1.Secret{}
			dstSecret := &corev1.Secret{}

			JustBeforeEach(func() {
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-main-" + rd.Name, Namespace: rd.Namespace}, mainSecret)
				}, maxWait, interval).Should(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-source-" + rd.Name, Namespace: rd.Namespace}, srcSecret)
				}, maxWait, interval).Should(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, dstSecret)
				}, maxWait, interval).Should(Succeed())
			})

			It("should have 4 entries in the main secret", func() {
				Expect(mainSecret.Data).To(HaveLen(4))
				Expect(mainSecret.Data).To(HaveKey("source"))
				Expect(mainSecret.Data).To(HaveKey("source.pub"))
				Expect(mainSecret.Data).To(HaveKey("destination"))
				Expect(mainSecret.Data).To(HaveKey("destination.pub"))
				Expect(mainSecret).To(beOwnedBy(rd))
			})
			It("should have 3 entries in the destination secret", func() {
				Expect(dstSecret.Data).To(HaveLen(3))
				keys := []string{"source.pub", "destination.pub", "destination"}
				for _, k := range keys {
					Expect(dstSecret.Data).To(HaveKeyWithValue(k, mainSecret.Data[k]))
				}
				Expect(dstSecret).To(beOwnedBy(rd))
			})
			It("should have 3 keys plus the connection address in the source secret", func() {
				Expect(srcSecret.Data).To(HaveLen(4))
				// ssh keys should match
				keys := []string{"source.pub", "source", "destination.pub"}
				for _, k := range keys {
					Expect(srcSecret.Data).To(HaveKeyWithValue(k, mainSecret.Data[k]))
				}
				// Should also have the connection address from the service. The default is a ClusterIP service
				svc := &corev1.Service{}
				Eventually(func() error {
					svcName := types.NamespacedName{
						Name:      "scribe-rsync-dest-" + rd.Name,
						Namespace: rd.Namespace,
					}
					return k8sClient.Get(ctx, svcName, svc)
				}, maxWait, interval).Should(Succeed())
				Expect(srcSecret.Data).To(HaveKeyWithValue("address", []uint8(svc.Spec.ClusterIP)))
				Expect(srcSecret).To(beOwnedBy(rd))
			})
			It("should be referenced in the ReplicationDestination status", func() {
				rdNew := &scribev1alpha1.ReplicationDestination{}
				Eventually(func() map[string]string {
					_ = k8sClient.Get(ctx, rdNsN, rdNew)
					return rdNew.Status.MethodStatus
				}, maxWait, interval).Should(HaveKeyWithValue(scribev1alpha1.RsyncConnectionInfoKey, srcSecret.Name))
			})
		})
	})
})
