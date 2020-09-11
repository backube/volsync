/*
Copyright 2020 The Scribe authors.

This file may be used, at your option, according to either the GNU AGPL 3.0 or
the Apache V2 license.

---
This program is free software: you can redistribute it and/or modify it under
the terms of the GNU Affero General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE.  See the GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License along
with this program.  If not, see <https://www.gnu.org/licenses/>.

---
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// +kubebuilder:validation:Required
package v1alpha1

import (
	"github.com/operator-framework/operator-lib/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceTriggerSpec defines when a volume will be synchronized with the
// destination.
type SourceTriggerSpec struct {
	// schedule is a cronspec (https://en.wikipedia.org/wiki/Cron#Overview) that
	// can be used to schedule replication to occur at regular, time-based
	// intervals.
	// +kubebuilder:validation:Pattern=`^(\d+|\*)(/\d+)?(\s+(\d+|\*)(/\d+)?){4}$`
	// +optional
	Schedule *string `json:"schedule,omitempty"`
}

// SourceSpec defines the desired state of Source
type SourceSpec struct {
	// source is the name of the PersistentVolumeClaim (PVC) to replicate.
	Source string `json:"source,omitempty"`
	// trigger determines when the latest state of the volume will be replicated
	// to the destination.
	Trigger SourceTriggerSpec `json:"trigger,omitempty"`
	// replicationMethod determines the method used to replicate the volume.
	// This may be either a method built into the Scribe controller or the name
	// of an external plugin. It must match the replicationMethod of the
	// corresponding destination.
	ReplicationMethod string `json:"replicationMethod,omitempty"`
	// parameters are method-specific key/value configuration parameters. For
	// more information, please see the documentation of the specific
	// replicationMethod being used.
	Parameters map[string]string `json:"parameters,omitempty"`
}

// SourceStatus defines the observed state of Source
type SourceStatus struct {
	// methodStatus provides status information that is specific to the
	// replicationMethod being used.
	MethodStatus map[string]string `json:"methodStatus,omitempty"`
	// conditions represent the latest available observations of the
	// source's state.
	Conditions status.Conditions `json:"conditions,omitempty"`
}

// Source defines the source for a replicated volume
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type Source struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of the Source, including the replication method
	// to use and its configuration.
	Spec SourceSpec `json:"spec,omitempty"`
	// status is the observed state of the Source as determined by the
	// controller.
	// +optional
	Status *SourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SourceList contains a list of Source
type SourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Source `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Source{}, &SourceList{})
}
