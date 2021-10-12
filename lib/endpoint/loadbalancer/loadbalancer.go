package loadbalancer

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/backube/volsync/lib/endpoint"

	"github.com/backube/volsync/lib/meta"
	"github.com/backube/volsync/lib/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type LoadBalancer struct {
	hostname       string
	ingressPort    int32
	backendPort    int32
	namespacedName types.NamespacedName
	objMeta        meta.ObjectMetaMutation
}

// Register should be used as soon as scheme is created to add
// route objects for encoding/decoding
func Register(scheme *runtime.Scheme) error {
	return nil
}

// APIsToWatch give a list of APIs to watch if using this package
// to deploy the endpoint
func APIsToWatch() ([]client.Object, error) {
	return []client.Object{&corev1.Service{}}, nil
}

// New creates a loadbalancer endpoint object, deploys the resources on  the cluster
// and then checks for the health of the loadbalancer. Before using the fields
// it is always recommended to check if the loadbalancer is healthy.
//
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
func New(ctx context.Context, c client.Client,
	name types.NamespacedName,
	metaMutation meta.ObjectMetaMutation,
	backendPort, ingressPort int32) (endpoint.Endpoint, error) {
	s := &LoadBalancer{
		namespacedName: name,
		objMeta:        metaMutation,
		backendPort:    backendPort,
		ingressPort:    ingressPort,
	}

	err := s.reconcileService(ctx, c)
	if err != nil {
		return nil, err
	}

	healthy, err := s.IsHealthy(ctx, c)
	if err != nil {
		return nil, err
	}

	if !healthy {
		return nil, fmt.Errorf("loadbalancer endpoint is not healthy")
	}

	return s, err
}

func (l *LoadBalancer) NamespacedName() types.NamespacedName {
	return l.namespacedName
}

func (l *LoadBalancer) Hostname() string {
	return l.hostname
}

func (l *LoadBalancer) BackendPort() int32 {
	return l.backendPort
}

func (l *LoadBalancer) IngressPort() int32 {
	return l.ingressPort
}

func (l *LoadBalancer) IsHealthy(ctx context.Context, c client.Client) (bool, error) {
	svc := &corev1.Service{}
	err := c.Get(ctx, l.NamespacedName(), svc)
	if err != nil {
		return false, err
	}

	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
			l.hostname = svc.Status.LoadBalancer.Ingress[0].Hostname
		}
		if svc.Status.LoadBalancer.Ingress[0].IP != "" {
			l.hostname = svc.Status.LoadBalancer.Ingress[0].IP
		}
		return true, nil
	}
	return false, nil
}

func (l *LoadBalancer) MarkForCleanup(ctx context.Context, c client.Client, key, value string) error {
	// mark service for deletion
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      l.namespacedName.Name,
			Namespace: l.namespacedName.Namespace,
		},
	}
	return utils.UpdateWithLabel(ctx, c, svc, key, value)
}

func (l *LoadBalancer) reconcileService(ctx context.Context, c client.Client) error {
	serviceSelector := l.objMeta.Labels()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      l.NamespacedName().Name,
			Namespace: l.NamespacedName().Namespace,
		},
	}

	// TODO: log the return operation from CreateOrUpdate
	_, err := controllerutil.CreateOrUpdate(ctx, c, service, func() error {
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:     l.NamespacedName().Name,
				Protocol: corev1.ProtocolTCP,
				Port:     l.IngressPort(),
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: l.BackendPort(),
				},
			},
		}
		service.Spec.Selector = serviceSelector
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Labels = l.objMeta.Labels()
		service.OwnerReferences = l.objMeta.OwnerReferences()
		return nil
	})

	return err
}
