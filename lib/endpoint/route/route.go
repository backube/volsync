package route

import (
	"context"
	"errors"
	"fmt"
	"github.com/backube/volsync/lib/endpoint"
	"github.com/backube/volsync/lib/meta"
	"github.com/backube/volsync/lib/utils"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metaapi "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// Register should be used as soon as scheme is created to add
// route objects for encoding/decoding
func Register(scheme *runtime.Scheme) error {
	return routev1.AddToScheme(scheme)
}

// APIsToWatch give a list of APIs to watch if using this package
// to deploy the endpoint. The error can be checked as follows to determine if
// the package is not usable with the given kube apiserver
//  	noResourceError := &metaapi.NoResourceMatchError{}
//		if errors.As(err, &noResourceError) {
// 		}
func APIsToWatch(c client.Client) ([]client.Object, error) {
	_, err := c.RESTMapper().ResourceFor(schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	})
	noResourceError := &metaapi.NoResourceMatchError{}
	if errors.As(err, &noResourceError) {
		return []client.Object{}, fmt.Errorf("route package unusable: %w", err)
	}
	if err != nil {
		return []client.Object{}, fmt.Errorf("unable to find the resource needed for this package")
	}
	return []client.Object{&routev1.Route{}, &corev1.Service{}}, nil
}

var IngressPort int32 = 443

type EndpointType string

type Route struct {
	hostname string

	port           int32
	endpointType   EndpointType
	namespacedName types.NamespacedName
	objMeta        meta.ObjectMetaMutation
}

// New creates the route endpoint object, deploys the resource on the cluster
// and then checks for the health of the route. Before using the fields of the route
// it is always recommended to check if the route is healthy.
//
// In order to identify if the route API exists check for the following error after calling
// New()
// noResourceError := &metaapi.NoResourceMatchError{}
//	switch {
//	case errors.As(err, &noResourceError):
//		// log route is not available, reconcilers should not requeue at this point
//		log.Info("route.openshift.io is unavailable, route endpoint will be disabled")
//  }
//
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
func New(ctx context.Context, c client.Client,
	namespacedName types.NamespacedName,
	eType EndpointType,
	metaMutation meta.ObjectMetaMutation) (endpoint.Endpoint, error) {
	if eType != EndpointTypePassthrough && eType != EndpointTypeInsecureEdge {
		return nil, fmt.Errorf("unsupported endpoint type for routes")
	}

	r := &Route{
		namespacedName: namespacedName,
		objMeta:        metaMutation,
		endpointType:   eType,
	}

	switch r.endpointType {
	case EndpointTypeInsecureEdge:
		r.port = int32(InsecureEdgeTerminationPolicyPort)
	case EndpointTypePassthrough:
		r.port = int32(TLSTerminationPassthroughPolicyPort)
	}

	err := r.reconcileServiceForRoute(ctx, c)
	if err != nil {
		return nil, err
	}

	err = r.reconcileRoute(ctx, c)
	if err != nil {
		return nil, err
	}

	healthy, err := r.IsHealthy(ctx, c)
	if err != nil {
		return nil, err
	}

	if !healthy {
		return nil, fmt.Errorf("route endpoint is not healthy")
	}

	return r, nil
}

func (r *Route) NamespacedName() types.NamespacedName {
	return r.namespacedName
}

func (r *Route) Hostname() string {
	return r.hostname
}

func (r *Route) BackendPort() int32 {
	return r.port
}

func (r *Route) IngressPort() int32 {
	return IngressPort
}

func (r *Route) IsHealthy(ctx context.Context, c client.Client) (bool, error) {
	route := &routev1.Route{}
	err := c.Get(ctx, r.NamespacedName(), route)
	if err != nil {
		return false, err
	}
	if route.Spec.Host == "" {
		return false, fmt.Errorf("hostname not set for route: %s", route)
	}

	if len(route.Status.Ingress) > 0 && len(route.Status.Ingress[0].Conditions) > 0 {
		for _, condition := range route.Status.Ingress[0].Conditions {
			if condition.Type == routev1.RouteAdmitted && condition.Status == corev1.ConditionTrue {
				// TODO: remove setHostname and configure the hostname after this condition has been satisfied,
				//  this is the implementation detail that we dont need the users of the interface work with
				err := r.setFields(ctx, c)
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

func (r *Route) MarkForCleanup(ctx context.Context, c client.Client, key, value string) error {
	// update service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.namespacedName.Name,
			Namespace: r.namespacedName.Namespace,
		},
	}
	err := utils.UpdateWithLabel(ctx, c, svc, key, value)
	if err != nil {
		return err
	}

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.namespacedName.Name,
			Namespace: r.namespacedName.Namespace,
		},
	}
	return utils.UpdateWithLabel(ctx, c, route, key, value)
}

func (r *Route) reconcileServiceForRoute(ctx context.Context, c client.Client) error {
	port := r.BackendPort()

	serviceSelector := r.objMeta.Labels()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
		},
	}

	// TODO: log the return operation from CreateOrUpdate
	_, err := controllerutil.CreateOrPatch(ctx, c, service, func() error {
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:     r.NamespacedName().Name,
				Protocol: corev1.ProtocolTCP,
				Port:     port,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: port,
				},
			},
		}

		service.Spec.Selector = serviceSelector
		service.Spec.Type = corev1.ServiceTypeClusterIP
		service.Labels = serviceSelector
		service.OwnerReferences = r.objMeta.OwnerReferences()
		return nil
	})

	return err
}

func (r *Route) reconcileRoute(ctx context.Context, c client.Client) error {
	termination := &routev1.TLSConfig{}
	switch r.endpointType {
	case EndpointTypeInsecureEdge:
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
	case EndpointTypePassthrough:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
	}

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, route, func() error {
		route.Spec.Port = &routev1.RoutePort{
			TargetPort: intstr.FromInt(int(r.port)),
		}
		route.Spec.To = routev1.RouteTargetReference{
			Kind: "Service",
			Name: r.NamespacedName().Name,
		}
		route.Spec.TLS = termination
		route.Labels = r.objMeta.Labels()
		route.OwnerReferences = r.objMeta.OwnerReferences()
		return nil
	})

	return err
}

func (r *Route) getRoute(ctx context.Context, c client.Client) (*routev1.Route, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: r.NamespacedName().Name, Namespace: r.NamespacedName().Namespace},
		route)
	if err != nil {
		return nil, err
	}
	return route, err
}

func (r *Route) setFields(ctx context.Context, c client.Client) error {
	route, err := r.getRoute(ctx, c)
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
