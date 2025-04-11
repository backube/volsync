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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/backube/volsync/internal/controller/utils"
)

type testLabelable struct {
	M map[string]string
}

func (t *testLabelable) GetLabels() map[string]string {
	return t.M
}
func (t *testLabelable) SetLabels(labels map[string]string) {
	t.M = labels
}
func newTestLabelable(m map[string]string) *testLabelable {
	if m == nil { // Preserve nil map
		return &testLabelable{}
	}
	t := testLabelable{M: map[string]string{}}
	for k, v := range m {
		t.M[k] = v
	}
	return &t
}

var _ utils.Labelable = &testLabelable{}

var _ = Describe("Label helpers", func() {
	var baseLabels map[string]string
	BeforeEach(func() {
		baseLabels = map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}
	})
	When("adding labels", func() {
		It("can add labels to an empty map", func() {
			t := newTestLabelable(nil)
			Expect(utils.AddLabel(t, "one", "two")).To(BeTrue())
			Expect(t.M).To(HaveLen(1))
			Expect(t.M).To(HaveKeyWithValue("one", "two"))
		})
		It("can add labels to an existing map", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddLabel(t, "one", "two")).To(BeTrue())
			Expect(t.M).To(HaveKeyWithValue("one", "two"))
			Expect(t.M).To(HaveLen(len(baseLabels) + 1))
		})
		It("overwrites existing values", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddLabel(t, "key3", "newvalue")).To(BeTrue())
			Expect(t.M).To(HaveKeyWithValue("key3", "newvalue"))
			Expect(t.M).To(HaveLen(len(baseLabels)))
		})
		It("returns false if no changes are made", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddLabel(t, "key1", "value1")).To(BeFalse())
			Expect(t.M).To(HaveKeyWithValue("key1", "value1"))
			Expect(t.M).To(HaveLen(len(baseLabels)))
		})
	})
	When("adding many labels", func() {
		var newLabels map[string]string
		BeforeEach(func() {
			newLabels = map[string]string{
				"new1": "newval1",
				"new2": "newval2",
			}
		})
		It("can add labels to an empty map", func() {
			t := newTestLabelable(nil)
			Expect(utils.AddAllLabels(t, newLabels)).To(BeTrue())
			Expect(t.M).To(HaveLen(len(newLabels)))
			Expect(t.M).To(HaveKeyWithValue("new1", "newval1"))
			Expect(t.M).To(HaveKeyWithValue("new2", "newval2"))
		})
		It("can add labels to an existing map", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddAllLabels(t, newLabels)).To(BeTrue())
			Expect(t.M).To(HaveKeyWithValue("new1", "newval1"))
			Expect(t.M).To(HaveKeyWithValue("new2", "newval2"))
			Expect(t.M).To(HaveLen(len(baseLabels) + len(newLabels)))
		})
		It("can handle an empty map", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddAllLabels(t, map[string]string{})).To(BeFalse())
			Expect(t.M).To(HaveLen(len(baseLabels)))
		})
		It("can handle a nil map", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.AddAllLabels(t, nil)).To(BeFalse())
			Expect(t.M).To(HaveLen(len(baseLabels)))
		})
	})
	When("removing labels", func() {
		It("can remove labels from a nil map", func() {
			t := newTestLabelable(nil)
			Expect(utils.RemoveLabel(t, "blah")).To(BeFalse())
			Expect(t.M).To(HaveLen(0))
		})
		It("can remove labels from an empty map", func() {
			t := newTestLabelable(map[string]string{})
			Expect(utils.RemoveLabel(t, "blah")).To(BeFalse())
			Expect(t.M).To(HaveLen(0))
		})
		It("can remove labels from a map w/o the specified key", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.RemoveLabel(t, "blah")).To(BeFalse())
			Expect(t.M).To(HaveLen(len(baseLabels)))
		})
		It("can remove labels from a map", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.RemoveLabel(t, "key2")).To(BeTrue())
			Expect(t.M).To(HaveLen(len(baseLabels) - 1))
			Expect(t.M).NotTo(HaveKey("key2"))
		})
		It("can remove the only label from a map", func() {
			t := newTestLabelable(map[string]string{"key1": "value1"})
			Expect(utils.RemoveLabel(t, "key1")).To(BeTrue())
			Expect(t.M).To(HaveLen(0))
		})
	})
	When("checking labels", func() {
		It("can check against a nil map", func() {
			t := newTestLabelable(nil)
			Expect(utils.HasLabel(t, "one")).To(BeFalse())
		})
		It("can check against an empty map", func() {
			t := newTestLabelable(map[string]string{})
			Expect(utils.HasLabel(t, "one")).To(BeFalse())
		})
		It("can check with the key missing", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.HasLabel(t, "one")).To(BeFalse())
		})
		It("can check with the key present", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.HasLabel(t, "key1")).To(BeTrue())
		})
		It("can check with the key/value missing", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.HasLabelWithValue(t, "one", "two")).To(BeFalse())
		})
		It("can check with the key present but value wrong", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.HasLabelWithValue(t, "key1", "WRONG")).To(BeFalse())
		})
		It("can check with the key/value present", func() {
			t := newTestLabelable(baseLabels)
			Expect(utils.HasLabelWithValue(t, "key1", "value1")).To(BeTrue())
		})
	})
	Context("VolSync ownership marking", func() {
		It("can mark and recognize objects", func() {
			pod := corev1.Pod{}
			Expect(utils.IsOwnedByVolsync(&pod)).To(BeFalse())

			Expect(utils.SetOwnedByVolSync(&pod)).To(BeTrue())
			Expect(utils.IsOwnedByVolsync(&pod)).To(BeTrue())

			Expect(utils.RemoveOwnedByVolSync(&pod)).To(BeTrue())
			Expect(utils.IsOwnedByVolsync(&pod)).To(BeFalse())
		})
	})
})
