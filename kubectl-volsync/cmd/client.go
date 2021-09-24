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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
