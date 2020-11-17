/*
Copyright 2020 The Scribe authors.

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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type rsyncSvcDescription struct {
	Context  context.Context
	Client   client.Client
	Scheme   *runtime.Scheme
	Service  *corev1.Service
	Owner    metav1.Object
	Type     *corev1.ServiceType
	Selector map[string]string
	Port     *int32
}

func (d *rsyncSvcDescription) reconcile(l logr.Logger) (bool, error) {
	logger := l.WithValues("service", nameFor(d.Service))

	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.Service, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.Service, d.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		d.Service.ObjectMeta.Annotations = map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
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
		d.Service.Spec.Ports[0].Name = "ssh"
		if d.Port != nil {
			d.Service.Spec.Ports[0].Port = *d.Port
		} else {
			d.Service.Spec.Ports[0].Port = 22
		}
		d.Service.Spec.Ports[0].Protocol = corev1.ProtocolTCP
		d.Service.Spec.Ports[0].TargetPort = intstr.FromInt(22)
		if d.Service.Spec.Type == corev1.ServiceTypeClusterIP {
			d.Service.Spec.Ports[0].NodePort = 0
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Service reconcile failed")
		return false, err
	}

	logger.V(1).Info("Service reconciled", "operation", op)
	return true, nil
}

func getServiceAddress(svc *corev1.Service) string {
	address := svc.Spec.ClusterIP
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
				address = svc.Status.LoadBalancer.Ingress[0].Hostname
			} else if svc.Status.LoadBalancer.Ingress[0].IP != "" {
				address = svc.Status.LoadBalancer.Ingress[0].IP
			}
		} else {
			address = ""
		}
	}
	return address
}
