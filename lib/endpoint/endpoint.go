package endpoint

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Endpoint knows how to connect with a Transport or a Transfer
type Endpoint interface {
	// NamespacedName returns a ns name to identify this endpoint
	NamespacedName() types.NamespacedName
	// Hostname returns a hostname for the endpoint
	Hostname() string
	// BackendPort returns a port which can be used by the application behind this endpoint
	BackendPort() int32
	// IngressPort is a port which is used by the clients to connect to the endpoint
	IngressPort() int32
	// IsHealthy returns whether or not all Kube resources used by endpoint are healthy
	IsHealthy(ctx context.Context, c client.Client) (bool, error)
	// MarkForCleanup
	MarkForCleanup(ctx context.Context, c client.Client, key, value string) error
}
