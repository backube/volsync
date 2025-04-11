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

package volumehandler

import (
	"errors"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/events"
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
	if vh.eventRecorder == nil {
		// FakeRecorder just discards Events
		vh.eventRecorder = &events.FakeRecorder{}
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
func WithOwner(o client.Object) VHOption {
	return func(vh *VolumeHandler) {
		vh.owner = o
	}
}

// From populates the VolumeHandler as a copy of an existing VolumeHandler
func From(v *VolumeHandler) VHOption {
	return func(vh *VolumeHandler) {
		*vh = *v
	}
}

// FromSource populates the VolumeHandler configuration based on the common
// source volume options
func FromSource(s *volsyncv1alpha1.ReplicationSourceVolumeOptions) VHOption {
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
func FromDestination(d *volsyncv1alpha1.ReplicationDestinationVolumeOptions) VHOption {
	return func(vh *VolumeHandler) {
		vh.copyMethod = d.CopyMethod
		vh.capacity = d.Capacity
		vh.storageClassName = d.StorageClassName
		vh.accessModes = d.AccessModes
		vh.volumeSnapshotClassName = d.VolumeSnapshotClassName
	}
}

func AccessModes(am []corev1.PersistentVolumeAccessMode) VHOption {
	return func(vh *VolumeHandler) {
		vh.accessModes = am
	}
}

func CopyMethod(cm volsyncv1alpha1.CopyMethodType) VHOption {
	return func(vh *VolumeHandler) {
		vh.copyMethod = cm
	}
}

func Capacity(c *resource.Quantity) VHOption {
	return func(vh *VolumeHandler) {
		vh.capacity = c
	}
}

func VolumeMode(vm *corev1.PersistentVolumeMode) VHOption {
	return func(vh *VolumeHandler) {
		vh.volumeMode = vm
	}
}

func StorageClassName(sc *string) VHOption {
	return func(vh *VolumeHandler) {
		vh.storageClassName = sc
	}
}

func VolumeSnapshotClassName(vsc *string) VHOption {
	return func(vh *VolumeHandler) {
		vh.volumeSnapshotClassName = vsc
	}
}

func WithRecorder(r events.EventRecorder) VHOption {
	return func(vh *VolumeHandler) {
		vh.eventRecorder = r
	}
}
