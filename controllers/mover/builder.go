/*
Copyright 2021 The VolSync authors.

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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var (
	ErrNoMoverFound        = fmt.Errorf("a replication method must be specified")
	ErrMultipleMoversFound = fmt.Errorf("only one replication method can be supplied")
)

// Catalog is the list of the available Builders for the controller to use when
// attempting to find an appropriate mover to service the RS/RD CR.
var Catalog []Builder

// Register should be called by each mover via an init function to register the
// mover w/ the main VolSync codebase.
func Register(builder Builder) {
	Catalog = append(Catalog, builder)
}

// Builder is used to construct Mover instances for the different data
// mover types.
type Builder interface {
	// FromSource attempts to construct a Mover from the provided
	// ReplicationSource. If the RS does not reference the Builder's mover type,
	// this function should return (nil, nil).
	FromSource(client client.Client, logger logr.Logger,
		eventRecorder events.EventRecorder,
		source *volsyncv1alpha1.ReplicationSource, privileged bool) (Mover, error)

	// FromDestination attempts to construct a Mover from the provided
	// ReplicationDestination. If the RS does not reference the Builder's mover
	// type, this function should return (nil, nil).
	FromDestination(client client.Client, logger logr.Logger,
		eventRecorder events.EventRecorder,
		destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (Mover, error)

	// VersionInfo returns a string describing the version of this mover. In
	// most cases, this is the container image/tag that will be used.
	VersionInfo() string
}

func GetDestinationMoverFromCatalog(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	destination *volsyncv1alpha1.ReplicationDestination, privileged bool) (Mover, error) {
	var dataMover Mover
	for _, builder := range Catalog {
		candidate, err := builder.FromDestination(client, logger, eventRecorder, destination, privileged)
		if err == nil && candidate != nil {
			if dataMover != nil {
				// Found 2 movers claiming this CR...
				return nil, ErrMultipleMoversFound
			}
			dataMover = candidate
		}
	}
	if dataMover == nil { // No mover matched
		return nil, ErrNoMoverFound
	}
	return dataMover, nil
}

func GetSourceMoverFromCatalog(client client.Client, logger logr.Logger,
	eventRecorder events.EventRecorder,
	source *volsyncv1alpha1.ReplicationSource, privileged bool) (Mover, error) {
	var dataMover Mover
	for _, builder := range Catalog {
		candidate, err := builder.FromSource(client, logger, eventRecorder, source, privileged)
		if err == nil && candidate != nil {
			if dataMover != nil {
				// Found 2 movers claiming this CR...
				return nil, ErrMultipleMoversFound
			}
			dataMover = candidate
		}
	}
	if dataMover == nil { // No mover matched
		return nil, ErrNoMoverFound
	}
	return dataMover, nil
}
