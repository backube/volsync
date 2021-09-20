package route

import (
	"context"
	"fmt"

	"github.com/backube/volsync/lib/endpoint"
	"github.com/backube/volsync/lib/meta"
	"github.com/backube/volsync/lib/utils"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	EndpointTypePassthrough             = "EndpointTypePassthrough"
	EndpointTypeInsecureEdge            = "EndpointTypeInsecureEdge"
	InsecureEdgeTerminationPolicyPort   = 8080
	TLSTerminationPassthroughPolicyPort = 6443
)

var IngressPort int32 = 443

type EndpointType string

type Endpoint struct {
	hostname string

	port           int32
	endpointType   EndpointType
	namespacedName types.NamespacedName
	objMeta        meta.ObjectMetaMutation
}

// NewEndpoint creates the route endpoint object, deploys the resource on the cluster
// and then checks for the health of the route. Before using the fields of the route
// it is always recommended to check if the route is healthy.
func NewEndpoint(c client.Client,
	namespacedName types.NamespacedName,
	eType EndpointType,
	metaMutation meta.ObjectMetaMutation) (endpoint.Endpoint, error) {
	err := routev1.AddToScheme(c.Scheme())
	if err != nil {
		return nil, err
	}

	if eType != EndpointTypePassthrough && eType != EndpointTypeInsecureEdge {
		panic("unsupported endpoint type for routes")
	}

	r := &Endpoint{
		namespacedName: namespacedName,
		objMeta:        metaMutation,
		endpointType:   eType,
	}

	err = r.reconcileServiceForRoute(c)
	if err != nil {
		return nil, err
	}

	err = r.reconcileRoute(c)
	if err != nil {
		return nil, err
	}

	healthy, err := r.IsHealthy(c)
	if err != nil {
		return nil, err
	}

	if !healthy {
		return nil, fmt.Errorf("route endpoint is not healthy")
	}

	return r, nil
}

func (r *Endpoint) NamespacedName() types.NamespacedName {
	return r.namespacedName
}

func (r *Endpoint) Hostname() string {
	return r.hostname
}

func (r *Endpoint) BackendPort() int32 {
	return r.port
}

func (r *Endpoint) IngressPort() int32 {
	return IngressPort
}

func (r *Endpoint) IsHealthy(c client.Client) (bool, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), r.NamespacedName(), route)
	if err != nil {
		return false, err
	}
	if route.Spec.Host == "" {
		return false, fmt.Errorf("hostname not set for rsync route: %s", route)
	}

	if len(route.Status.Ingress) > 0 && len(route.Status.Ingress[0].Conditions) > 0 {
		for _, condition := range route.Status.Ingress[0].Conditions {
			if condition.Type == routev1.RouteAdmitted && condition.Status == corev1.ConditionTrue {
				// TODO: remove setHostname and configure the hostname after this condition has been satisfied,
				//  this is the implementation detail that we dont need the users of the interface work with
				err := r.setFields(c)
				if err != nil {
					return true, err
				}
				return true, nil
			}
		}
	}
	// TODO: probably using error.Wrap/Unwrap here makes much more sense
	return false, fmt.Errorf("route status is not in valid state: %s", route.Status)
}

func (r *Endpoint) MarkForCleanup(c client.Client, key, value string) error {
	// update service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.namespacedName.Name,
			Namespace: r.namespacedName.Namespace,
		},
	}
	err := utils.UpdateWithLabel(c, svc, key, value)
	if err != nil {
		return err
	}

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.namespacedName.Name,
			Namespace: r.namespacedName.Namespace,
		},
	}
	return utils.UpdateWithLabel(c, route, key, value)
}

func (r *Endpoint) reconcileServiceForRoute(c client.Client) error {
	port := r.BackendPort()

	serviceSelector := r.objMeta.Labels()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
		},
	}

	// TODO: log the return operation from CreateOrUpdate
	_, err := controllerutil.CreateOrUpdate(context.TODO(), c, service, func() error {
		if service.CreationTimestamp.IsZero() {
			service.Spec = corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     r.NamespacedName().Name,
						Protocol: corev1.ProtocolTCP,
						Port:     port,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: port,
						},
					},
				},
				Selector: serviceSelector,
				Type:     corev1.ServiceTypeClusterIP,
			}
		}

		service.Labels = r.objMeta.Labels()
		service.OwnerReferences = r.objMeta.OwnerReferences()
		return nil
	})

	return err
}

func (r *Endpoint) reconcileRoute(c client.Client) error {
	termination := &routev1.TLSConfig{}
	switch r.endpointType {
	case EndpointTypeInsecureEdge:
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
		r.port = int32(InsecureEdgeTerminationPolicyPort)
	case EndpointTypePassthrough:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
		r.port = int32(TLSTerminationPassthroughPolicyPort)
	}

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(context.TODO(), c, route, func() error {
		if route.CreationTimestamp.IsZero() {
			route.Spec = routev1.RouteSpec{
				Port: &routev1.RoutePort{
					TargetPort: intstr.FromInt(int(r.port)),
				},
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: r.NamespacedName().Name,
				},
				TLS: termination,
			}
		}
		route.Labels = r.objMeta.Labels()
		route.OwnerReferences = r.objMeta.OwnerReferences()
		return nil
	})

	return err
}

func (r *Endpoint) getRoute(c client.Client) (*routev1.Route, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: r.NamespacedName().Name, Namespace: r.NamespacedName().Namespace},
		route)
	if err != nil {
		return nil, err
	}
	return route, err
}

func (r *Endpoint) setFields(c client.Client) error {
	route, err := r.getRoute(c)
	if err != nil {
		return err
	}

	if route.Spec.Host == "" {
		return fmt.Errorf("route %s has empty spec.host field", r.NamespacedName())
	}
	if route.Spec.Port == nil {
		return fmt.Errorf("route %s has empty spec.port field", r.NamespacedName())
	}

	r.hostname = route.Spec.Host

	r.port = route.Spec.Port.TargetPort.IntVal

	switch route.Spec.TLS.Termination {
	case routev1.TLSTerminationEdge:
		r.endpointType = EndpointTypeInsecureEdge
	case routev1.TLSTerminationPassthrough:
		r.endpointType = EndpointTypePassthrough
	case routev1.TLSTerminationReencrypt:
		return fmt.Errorf("route %s has unsupported spec.spec.tls.termination value", r.NamespacedName())
	}

	return nil
}
