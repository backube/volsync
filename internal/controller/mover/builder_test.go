/*
Copyright 2026 The VolSync authors.

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

package mover

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// fakeBuilder returns a fixed (Mover, error) pair for both RS and RD.
type fakeBuilder struct {
	mover Mover
	err   error
}

func (f *fakeBuilder) Name() string        { return "fake" }
func (f *fakeBuilder) VersionInfo() string { return "fake" }
func (f *fakeBuilder) FromSource(client.Client, logr.Logger, events.EventRecorder,
	*volsyncv1alpha1.ReplicationSource, bool) (Mover, error) {
	return f.mover, f.err
}
func (f *fakeBuilder) FromDestination(client.Client, logr.Logger, events.EventRecorder,
	*volsyncv1alpha1.ReplicationDestination, bool) (Mover, error) {
	return f.mover, f.err
}

type fakeMover struct{ Mover }

func TestGetMoverFromCatalogBuilderErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		builders []Builder
		wantErr  error
	}{
		{name: "no builders", wantErr: ErrNoMoverFound},
		{name: "no match", builders: []Builder{&fakeBuilder{}}, wantErr: ErrNoMoverFound},
		{name: "builder error is propagated", builders: []Builder{&fakeBuilder{err: boom}}, wantErr: boom},
		{
			name:     "builder error wins over a later match",
			builders: []Builder{&fakeBuilder{err: boom}, &fakeBuilder{mover: fakeMover{}}},
			wantErr:  boom,
		},
		{
			name:     "multiple matches",
			builders: []Builder{&fakeBuilder{mover: fakeMover{}}, &fakeBuilder{mover: fakeMover{}}},
			wantErr:  ErrMultipleMoversFound,
		},
		{name: "single match", builders: []Builder{&fakeBuilder{mover: fakeMover{}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := Catalog
			t.Cleanup(func() { Catalog = saved })
			Catalog = tc.builders

			m, err := GetSourceMoverFromCatalog(nil, logr.Discard(), nil, nil, false)
			checkCatalogResult(t, "source", m, err, tc.wantErr)
			m, err = GetDestinationMoverFromCatalog(nil, logr.Discard(), nil, nil, false)
			checkCatalogResult(t, "destination", m, err, tc.wantErr)
		})
	}
}

func checkCatalogResult(t *testing.T, kind string, m Mover, err, wantErr error) {
	t.Helper()
	if !errors.Is(err, wantErr) {
		t.Errorf("%s: got err %v, want %v", kind, err, wantErr)
	}
	if (wantErr == nil) != (m != nil) {
		t.Errorf("%s: got mover %v with wantErr %v", kind, m, wantErr)
	}
}
