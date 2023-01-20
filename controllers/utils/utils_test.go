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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("utils tests", func() {
	Describe("PvcIsReadOnly", func() {
		var pvc *corev1.PersistentVolumeClaim

		storageClassName := "mytest-storage-class"

		BeforeEach(func() {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc-1",
					Namespace: "test-pvc-1-namespace",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			}
		})

		When("PVC accessModes is set to only ROX", func() {
			BeforeEach(func() {
				pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
			})

			When("pvc.status.accessmodes is not defined", func() {
				It("Should determine the pvc is read-only from the pvc spec", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeTrue())
				})
			})

			When("pvc.status.accessmodes is defined", func() {
				BeforeEach(func() {
					pvc.Status.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}

					// Clear out the spec just to ensure the code gets the value from the status first
					pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{}
				})

				It("Should determine the pvc is read-only from the pvc status", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeTrue())
				})
			})
		})

		When("PVC access modes contains any writable access mode", func() {
			BeforeEach(func() {
				pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
					corev1.ReadOnlyMany, // Even if ROX is here, we should return readOnly is false
				}
			})

			When("pvc.status.accessmodes is not defined", func() {
				It("Should determine the pvc is not read-only from the pvc spec", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeFalse())
				})
			})

			When("pvc.status.accessmodes is defined", func() {
				BeforeEach(func() {
					pvc.Status.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
				})

				It("Should determine the pvc is not read-only from the pvc status", func() {
					Expect(utils.PvcIsReadOnly(pvc)).To(BeFalse())
				})
			})
		})
	})
})
