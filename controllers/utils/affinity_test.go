/*
Copyright 2022 The VolSync authors.

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

package utils_test

import (
	"github.com/backube/volsync/controllers/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Volume affinity", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
	var ns *corev1.Namespace
	var rwxPVC, rwoBoth, rwoPending, rwoNone, vsOnly *corev1.PersistentVolumeClaim
	var runningPod, pendingPod, vsPod *corev1.Pod

	makePVC := func(name string, mode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					mode,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
		pvc.Status.AccessModes = pvc.Spec.AccessModes
		Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())
		return pvc
	}

	makePod := func(name string, PVCs []corev1.PersistentVolumeClaim, phase corev1.PodPhase, isVolsync bool) *corev1.Pod {
		uniqueNodeName := name + "-" + ns.Name + "-node" // Make a unique node name based on the ns & pod name

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns.Name,
			},
			Spec: corev1.PodSpec{
				NodeName: uniqueNodeName,
				Tolerations: []corev1.Toleration{
					{
						Key:    name + "-key",
						Value:  "thevalue",
						Effect: corev1.TaintEffectNoExecute,
					},
				},
				Containers: []corev1.Container{{
					Name:  "name",
					Image: "image",
				}},
			},
		}
		for _, p := range PVCs {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: p.Name + "-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: p.Name,
					},
				},
			})
		}
		if isVolsync {
			utils.SetOwnedByVolSync(pod)
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.Phase = phase
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		return pod
	}

	makeNode := func(name string) *corev1.Node {
		// Make a fake node to simulate the one the pod is running on
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"kubernetes.io/hostname": name,
				},
			},
			Spec: corev1.NodeSpec{},
		}
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		return node
	}

	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "affinity-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())

		// Make PVCs...
		// Used only by running pod
		rwxPVC = makePVC("rwx", corev1.ReadWriteMany)
		// Used by both running and pending pod
		rwoBoth = makePVC("rwo-both", corev1.ReadWriteOnce)
		// Used only by pending pod
		rwoPending = makePVC("rwo-pending", corev1.ReadWriteOnce)
		// Not used by any pods
		rwoNone = makePVC("rwo-none", corev1.ReadWriteOnce)
		// Only used by a VolSync-owned Pod
		vsOnly = makePVC("vs-only", corev1.ReadWriteOnce)

		// Make Pods
		runningPod = makePod("running",
			[]corev1.PersistentVolumeClaim{*rwxPVC, *rwoBoth},
			corev1.PodRunning,
			false)
		pendingPod = makePod("pending",
			[]corev1.PersistentVolumeClaim{*rwoBoth, *rwoPending},
			corev1.PodPending,
			false)
		vsPod = makePod("vs",
			[]corev1.PersistentVolumeClaim{*rwoBoth, *vsOnly},
			corev1.PodRunning,
			true)
	})
	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())

	})

	Context("When no pod is found (and no node lookups required) using the PVC", func() {
		When("a PVC is RWX", func() {
			It("will have an empty (unrestricted) affinity", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwxPVC)
				Expect(err).NotTo(HaveOccurred())
				Expect(ai.NodeSelector).To(BeEmpty())
				Expect(ai.Tolerations).To(BeEmpty())
			})
		})

		When("an invalid pvc is specified", func() {
			It("will return an error", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, nil)
				Expect(err).To(HaveOccurred())
				Expect(ai).To(BeNil())
			})
		})

		When("a PVC is not in use", func() {
			It("will have an empty (unrestricted) affinity", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwoNone)
				Expect(err).NotTo(HaveOccurred())
				Expect(ai.NodeSelector).To(BeEmpty())
				Expect(ai.Tolerations).To(BeEmpty())
			})
		})
	})

	Context("When a pod is using the PVC", func() {
		// These tests will require looking up the node hosting the pod to find the proper nodeSelector
		var runningPodNode, pendingPodNode, vsPodNode *corev1.Node

		BeforeEach(func() {
			// create fake nodes that will "host" the associated pod
			runningPodNode = makeNode(runningPod.Spec.NodeName)
			pendingPodNode = makeNode(pendingPod.Spec.NodeName)
			vsPodNode = makeNode(vsPod.Spec.NodeName)
		})

		AfterEach(func() {
			// Nodes are global resources so try to clean up all before checking for errs
			node1Err := k8sClient.Delete(ctx, runningPodNode)
			node2Err := k8sClient.Delete(ctx, pendingPodNode)
			node3Err := k8sClient.Delete(ctx, vsPodNode)

			Expect(node1Err).NotTo(HaveOccurred())
			Expect(node2Err).NotTo(HaveOccurred())
			Expect(node3Err).NotTo(HaveOccurred())
		})

		When("a PVC is only being used by a Pending pod", func() {
			It("will have an affinity that matches that pod", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwoPending)
				Expect(err).NotTo(HaveOccurred())
				Expect(ai.NodeSelector).To(Equal(
					map[string]string{
						"kubernetes.io/hostname": pendingPod.Spec.NodeName,
					},
				))
				Expect(ai.Tolerations).To(Equal(pendingPod.Spec.Tolerations))
			})
		})

		When("a PVC is being used by a Running pod", func() {
			It("will have an affinity that matches that pod", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwoBoth)
				Expect(err).NotTo(HaveOccurred())
				Expect(ai.NodeSelector).To(Equal(
					map[string]string{
						"kubernetes.io/hostname": runningPod.Spec.NodeName,
					},
				))
				Expect(ai.Tolerations).To(Equal(runningPod.Spec.Tolerations))
			})

			When("The hosting node name does not match the hostname label", func() {
				var differentHostname string

				BeforeEach(func() {
					// Update the node to set the hostname label to something else
					differentHostname = runningPodNode.GetName() + "-extrastuffhere"

					runningPodNode.Labels["kubernetes.io/hostname"] = differentHostname

					Expect(k8sClient.Update(ctx, runningPodNode)).To(Succeed())
				})

				It("The affinity node selector should still be set properly to match the hostname label", func() {
					ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwoBoth)
					Expect(err).NotTo(HaveOccurred())
					Expect(ai.NodeSelector).To(Equal(
						map[string]string{
							"kubernetes.io/hostname": differentHostname,
						},
					))
					Expect(ai.Tolerations).To(Equal(runningPod.Spec.Tolerations))
				})
			})

			When("the hosting node does not have the kubernetes hostname label", func() {
				// Hopefully this never actually happens in a real cluster
				BeforeEach(func() {
					// remove the label
					delete(runningPodNode.Labels, "kubernetes.io/hostname")
					Expect(k8sClient.Update(ctx, runningPodNode)).To(Succeed())
				})

				It("Getting affinity should fail", func() {
					_, err := utils.AffinityFromVolume(ctx, k8sClient, logger, rwoBoth)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to find kubernetes.io/hostname label from node"))
				})
			})
		})

		// Disabled since the code was removed. VolSync ignores its own pods now
		XWhen("a PVC is being used only by a VolSync-owned pod", func() {
			It("will have an affinity that matches that pod", func() {
				ai, err := utils.AffinityFromVolume(ctx, k8sClient, logger, vsOnly)
				Expect(err).NotTo(HaveOccurred())
				Expect(ai.NodeSelector).To(Equal(
					map[string]string{
						"kubernetes.io/hostname": vsPod.Spec.NodeName,
					},
				))
				Expect(ai.Tolerations).To(Equal(vsPod.Spec.Tolerations))
			})
		})
	})
})
