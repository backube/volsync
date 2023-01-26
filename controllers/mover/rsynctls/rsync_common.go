/*
Copyright 2020 The VolSync authors.

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

package rsynctls

import (
	"context"

	"github.com/backube/volsync/controllers/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type rsyncSvcDescription struct {
	Context     context.Context
	Client      client.Client
	Service     *corev1.Service
	Owner       metav1.Object
	Type        *corev1.ServiceType
	Selector    map[string]string
	Port        *int32
	Annotations map[string]string
}

func (d *rsyncSvcDescription) Reconcile(l logr.Logger) error {
	logger := l.WithValues("service", client.ObjectKeyFromObject(d.Service))

	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.Service, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.Service, d.Client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(d.Service)

		if d.Service.ObjectMeta.Annotations == nil {
			d.Service.ObjectMeta.Annotations = map[string]string{}
		}
		if d.Annotations == nil {
			// Set our default annotations
			d.Service.ObjectMeta.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"] = "nlb"
		} else {
			// Use user-supplied annotations - do not replace Annotations entirely in case of system-added annotations
			updateMap(d.Service.ObjectMeta.Annotations, d.Annotations)
		}

		if d.Type != nil {
			d.Service.Spec.Type = *d.Type
		} else {
			d.Service.Spec.Type = corev1.ServiceTypeClusterIP
		}
		d.Service.Spec.Selector = d.Selector
		if len(d.Service.Spec.Ports) != 1 {
			d.Service.Spec.Ports = []corev1.ServicePort{{}}
		}
		d.Service.Spec.Ports[0].Name = "rsync-tls"
		if d.Port != nil {
			d.Service.Spec.Ports[0].Port = *d.Port
		} else {
			d.Service.Spec.Ports[0].Port = 8000
		}
		d.Service.Spec.Ports[0].Protocol = corev1.ProtocolTCP
		d.Service.Spec.Ports[0].TargetPort = intstr.FromInt(tlsContainerPort)
		if d.Service.Spec.Type == corev1.ServiceTypeClusterIP {
			d.Service.Spec.Ports[0].NodePort = 0
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Service reconcile failed")
		return err
	}

	logger.V(1).Info("Service reconciled", "operation", op)
	return nil
}

// Update map1 with any k,v pairs from map2
func updateMap(map1, map2 map[string]string) {
	for k, v := range map2 {
		map1[k] = v
	}
}
