//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("discoverSourcePVC unit tests", func() {
	var (
		logger logr.Logger
		scheme *runtime.Scheme
		kb     *Builder
	)

	BeforeEach(func() {
		logger = logr.Discard()
		scheme = runtime.NewScheme()
		Expect(volsyncv1alpha1.AddToScheme(scheme)).To(Succeed())
		kb = &Builder{}
	})

	DescribeTable("discoverSourcePVC",
		func(sourceName, sourceNamespace string, objects []client.Object, setupMock func() client.Client, expectedPVC string) {
			var c client.Client
			if setupMock != nil {
				c = setupMock()
			} else {
				c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			}

			result := kb.discoverSourcePVC(c, sourceName, sourceNamespace, logger)
			Expect(result).To(Equal(expectedPVC))
		},
		Entry("returns empty when source name is empty",
			"",
			"test-ns",
			[]client.Object{},
			nil,
			"",
		),
		Entry("returns empty when source namespace is empty",
			"test-source",
			"",
			[]client.Object{},
			nil,
			"",
		),
		Entry("returns empty when ReplicationSource not found",
			"nonexistent",
			"test-ns",
			[]client.Object{},
			nil,
			"",
		),
		Entry("returns empty when ReplicationSource doesn't use Kopia",
			"rsync-source",
			"test-ns",
			[]client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rsync-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Rsync:     &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
					},
				},
			},
			nil,
			"",
		),
		Entry("returns PVC when ReplicationSource uses Kopia",
			"kopia-source",
			"test-ns",
			[]client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "my-data-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo",
						},
					},
				},
			},
			nil,
			"my-data-pvc",
		),
		Entry("returns empty when Kopia source has no PVC",
			"kopia-source-no-pvc",
			"test-ns",
			[]client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-source-no-pvc",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo",
						},
					},
				},
			},
			nil,
			"",
		),
		Entry("handles permission errors gracefully",
			"test-source",
			"test-ns",
			[]client.Object{},
			func() client.Client {
				return &mockClientWithErrorUnit{
					err: kerrors.NewForbidden(
						schema.GroupResource{Group: "volsync.backube", Resource: "replicationsources"},
						"test-source",
						errors.New("user cannot get resource"),
					),
				}
			},
			"",
		),
	)
})

// mockClientWithErrorUnit is a mock client that returns a specific error for unit tests
type mockClientWithErrorUnit struct {
	client.Client
	err error
}

func (m *mockClientWithErrorUnit) Get(
	_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption,
) error {
	return m.err
}
