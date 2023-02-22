/*
Copyright 2020 The VolSync authors.

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

import corev1 "k8s.io/api/core/v1"

/********************************************************************
 * Replication source types
 ********************************************************************/

type ReplicationSourceRsyncTLSSpec struct {
	ReplicationSourceVolumeOptions `json:",inline"`
	// keySecret is the name of a Secret that contains the TLS pre-shared key to
	// be used for authentication. If not provided, the key will be generated.
	//+optional
	KeySecret *string `json:"keySecret,omitempty"`
	// address is the remote address to connect to for replication.
	//+optional
	Address *string `json:"address,omitempty"`
	// port is the port to connect to for replication. Defaults to 8000.
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationSource.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

type ReplicationSourceRsyncTLSStatus struct {
	// keySecret is the name of a Secret that contains the TLS pre-shared key to
	// be used for authentication. If not provided in .spec.rsyncTLS.keySecret,
	// the key Secret will be generated and named here.
	//+optional
	KeySecret *string `json:"keySecret,omitempty"`
}

/********************************************************************
 * Replication destination types
 ********************************************************************/

type ReplicationDestinationRsyncTLSSpec struct {
	ReplicationDestinationVolumeOptions `json:",inline"`
	// keySecret is the name of a Secret that contains the TLS pre-shared key to
	// be used for authentication. If not provided, the key will be generated.
	//+optional
	KeySecret *string `json:"keySecret,omitempty"`
	// serviceType determines the Service type that will be created for incoming
	// TLS connections.
	//+optional
	ServiceType *corev1.ServiceType `json:"serviceType,omitempty"`
	// serviceAnnotations defines annotations that will be added to the
	// service created for incoming SSH connections.  If set, these annotations
	// will be used instead of any VolSync default values.
	//+optional
	ServiceAnnotations *map[string]string `json:"serviceAnnotations,omitempty"`
	// MoverSecurityContext allows specifying the PodSecurityContext that will
	// be used by the data mover
	MoverSecurityContext *corev1.PodSecurityContext `json:"moverSecurityContext,omitempty"`
	// MoverServiceAccount allows specifying the name of the service account
	// that will be used by the data mover. This should only be used by advanced
	// users who want to override the service account normally used by the mover.
	// The service account needs to exist in the same namespace as the ReplicationDestination.
	//+optional
	MoverServiceAccount *string `json:"moverServiceAccount,omitempty"`
}

type ReplicationDestinationRsyncTLSStatus struct {
	// keySecret is the name of a Secret that contains the TLS pre-shared key to
	// be used for authentication. If not provided in .spec.rsyncTLS.keySecret,
	// the key Secret will be generated and named here.
	//+optional
	KeySecret *string `json:"keySecret,omitempty"`
	// address is the address to connect to for incoming TLS connections.
	//+optional
	Address *string `json:"address,omitempty"`
	// port is the port to connect to for incoming replication connections.
	//+optional
	Port *int32 `json:"port,omitempty"`
}
