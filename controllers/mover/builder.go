/*
Copyright 2021 The Scribe authors.

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
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

// Catalog is the list of the available Builders for the controller to use when
// attempting to find an appropriate mover to service the RS/RD CR.
var Catalog []Builder

// Register should be called by each mover via an init function to register the
// mover w/ the main Scribe codebase.
func Register(builder Builder) {
	Catalog = append(Catalog, builder)
}

// Builder is used to construct Mover instances for the different data
// mover types.
type Builder interface {
	// Initialize is called once at operator startup to allow the Builder to
	// initialize any necessary state such as registering command line options.
	// It is called before main() begins executing.
	Initialize()

	// FromSource attempts to construct a Mover from the provided
	// ReplicationSource. If the RS does not reference the Builder's mover type,
	// this function should return (nil, nil).
	FromSource(client client.Client, logger logr.Logger,
		source *scribev1alpha1.ReplicationSource) (Mover, error)

	// FromDestination attempts to construct a Mover from the provided
	// ReplicationDestination. If the RS does not reference the Builder's mover
	// type, this function should return (nil, nil).
	FromDestination(client client.Client, logger logr.Logger,
		destination *scribev1alpha1.ReplicationDestination) (Mover, error)
}
