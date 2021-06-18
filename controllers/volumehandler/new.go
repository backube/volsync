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

package volumehandler

import (
	"errors"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VHOption functions allow configuration of the VolumeHandler
type VHOption func(*VolumeHandler)

// NewVolumeHandler creates a VolumeHandler based on an ordered list of options.
func NewVolumeHandler(options ...VHOption) (*VolumeHandler, error) {
	vh := &VolumeHandler{}
	for _, option := range options {
		option(vh)
	}
	if vh.owner == nil {
		return nil, errors.New("an owner must be specified the VolumeHandler")
	}
	if vh.client == nil {
		return nil, errors.New("a Client must be provided")
	}
	return vh, nil
}

// WithClient specifies the Kubernetes client to use
func WithClient(c client.Client) VHOption {
	return func(vh *VolumeHandler) {
		vh.client = c
	}
}

// WithOwner specifies the Object should be the owner of Objects created by the
// VolumeHandler
func WithOwner(o v1.Object) VHOption {
	return func(vh *VolumeHandler) {
		vh.owner = o
	}
}

// FromSource populates the VolumeHandler configuration based on the common
// source volume options
func FromSource(s *scribev1alpha1.ReplicationSourceVolumeOptions) VHOption {
	return func(vh *VolumeHandler) {
		vh.copyMethod = s.CopyMethod
		vh.capacity = s.Capacity
		vh.storageClassName = s.StorageClassName
		vh.accessModes = s.AccessModes
		vh.volumeSnapshotClassName = s.VolumeSnapshotClassName
	}
}

// FromDestination populates the VolumeHandler configuration based on the common
// destination volume options
func FromDestination(d *scribev1alpha1.ReplicationDestinationVolumeOptions) VHOption {
	return func(vh *VolumeHandler) {
		vh.copyMethod = d.CopyMethod
		vh.capacity = d.Capacity
		vh.storageClassName = d.StorageClassName
		vh.accessModes = d.AccessModes
		vh.volumeSnapshotClassName = d.VolumeSnapshotClassName
	}
}
