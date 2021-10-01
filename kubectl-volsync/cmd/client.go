/*
Copyright © 2021 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// Get a new Client to access a kube cluster, specifying the cluster context to
// use or "" to use the default context.
func newClient(kubeContext string) (client.Client, error) {
	// configFlags := genericclioptions.NewConfigFlags(true)
	// if len(kubeContext) > 0 {
	// 	configFlags.Context = &kubeContext
	// }
	// factory := kcmdutil.NewFactory(configFlags)
	// clientConfig, err := factory.ToRESTConfig()
	clientConfig, err := config.GetConfigWithContext(kubeContext)
	if err != nil {
		return nil, err
	}
	// Add the Schemes for the types we'll need to access
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := volsyncv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return client.New(clientConfig, client.Options{Scheme: scheme})
}

// XClusterName is the equivlent of NamespacedName, but also containing a
// cluster context
type XClusterName struct {
	Cluster              string
	types.NamespacedName `mapstructure:",squash"`
}

// Parses a string of the format [context/]namespace/name into an XClusterName
func ParseXClusterName(name string) (*XClusterName, error) {
	components := strings.Split(name, "/")
	if len(components) == 3 {
		return &XClusterName{
			Cluster: components[0],
			NamespacedName: types.NamespacedName{
				Namespace: components[1],
				Name:      components[2],
			},
		}, nil
	} else if len(components) == 2 {
		return &XClusterName{
			NamespacedName: types.NamespacedName{
				Namespace: components[0],
				Name:      components[1],
			},
		}, nil
	}
	return nil, fmt.Errorf("name is not in the format [context/]namespace/objname: %s", name)
}

func XClusterNameFromRelationship(relation *Relationship, name string) (*XClusterName, error) {
	if !relation.IsSet(name) {
		return nil, fmt.Errorf("key %s not found in relationship", name)
	}
	var obj XClusterName
	if err := relation.UnmarshalKey(name, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}
