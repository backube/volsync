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

package utils

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DefaultSCCName is the default name of the volsync security context constraint
const DefaultSCCName = "volsync-privileged-mover" // #nosec G101 - gosec thinks this is a credential

// SCCName is the name of the SCC to use for the mover Jobs
var SCCName string

type SAHandler interface {
	Reconcile(ctx context.Context, l logr.Logger) (*corev1.ServiceAccount, error)
}

type SAHandlerVolSync struct {
	Context     context.Context
	Client      client.Client
	SA          *corev1.ServiceAccount
	Owner       metav1.Object
	Privileged  bool
	role        *rbacv1.Role
	roleBinding *rbacv1.RoleBinding
}

var _ SAHandler = &SAHandlerVolSync{}

type SAHandlerUserSupplied struct {
	Client client.Client
	SA     *corev1.ServiceAccount
}

var _ SAHandler = &SAHandlerUserSupplied{}

func NewSAHandler(c client.Client, owner metav1.Object, isSource,
	privileged bool, userSuppliedSA *string) SAHandler {
	if userSuppliedSA == nil {
		dir := "src"
		if !isSource {
			dir = "dst"
		}
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "volsync-" + dir + "-" + owner.GetName(),
				Namespace: owner.GetNamespace(),
			},
		}
		return &SAHandlerVolSync{
			Client:     c,
			SA:         sa,
			Owner:      owner,
			Privileged: privileged,
		}
	}

	// User has supplised a moverSecurityContext - use SAHandlerUserSupplied to ensure it exists
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *userSuppliedSA,
			Namespace: owner.GetNamespace(),
		},
	}
	return &SAHandlerUserSupplied{
		Client: c,
		SA:     sa,
	}
}

func (d *SAHandlerVolSync) Reconcile(ctx context.Context, l logr.Logger) (*corev1.ServiceAccount, error) {
	d.Context = ctx
	cont, err := ReconcileBatch(l,
		d.ensureSA,
		d.ensureRole,
		d.ensureRoleBinding,
	)
	if cont {
		return d.SA, err
	}
	return nil, err
}

func (d *SAHandlerVolSync) ensureSA(l logr.Logger) (bool, error) {
	logger := l.WithValues("ServiceAccount", client.ObjectKeyFromObject(d.SA))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.SA, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.SA, d.Client.Scheme()); err != nil {
			logger.Error(err, ErrUnableToSetControllerRef)
			return err
		}
		SetOwnedByVolSync(d.SA)
		return nil
	})
	if err != nil {
		logger.Error(err, "ServiceAccount reconcile failed")
		return false, err
	}

	logger.V(1).Info("ServiceAccount reconciled", "operation", op)
	return true, nil
}

func (d *SAHandlerVolSync) ensureRole(l logr.Logger) (bool, error) {
	d.role = &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.SA.Name,
			Namespace: d.SA.Namespace,
		},
	}
	logger := l.WithValues("Role", client.ObjectKeyFromObject(d.role))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.role, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.role, d.Client.Scheme()); err != nil {
			logger.Error(err, ErrUnableToSetControllerRef)
			return err
		}
		SetOwnedByVolSync(d.role)
		if d.Privileged { // Only grant SCC to privileged movers
			d.role.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{"security.openshift.io"},
					Resources: []string{"securitycontextconstraints"},
					// Must match the name of the SCC that is deployed w/ the operator
					// config/openshift/mover_scc.yaml
					ResourceNames: []string{SCCName},
					Verbs:         []string{"use"},
				},
			}
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Role reconcile failed")
		return false, err
	}

	logger.V(1).Info("Role reconciled", "operation", op)
	return true, nil
}

func (d *SAHandlerVolSync) ensureRoleBinding(l logr.Logger) (bool, error) {
	d.roleBinding = &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.SA.Name,
			Namespace: d.SA.Namespace,
		},
	}
	logger := l.WithValues("RoleBinding", client.ObjectKeyFromObject(d.roleBinding))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.roleBinding, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.roleBinding, d.Client.Scheme()); err != nil {
			logger.Error(err, ErrUnableToSetControllerRef)
			return err
		}
		SetOwnedByVolSync(d.roleBinding)
		d.roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     d.role.Name,
		}
		d.roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      d.SA.Name,
				Namespace: d.SA.Namespace,
			},
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "RoleBinding reconcile failed")
		return false, err
	}

	logger.V(1).Info("RoleBinding reconciled", "operation", op)
	return true, nil
}

func (d *SAHandlerUserSupplied) Reconcile(ctx context.Context, l logr.Logger) (*corev1.ServiceAccount, error) {
	// User supplied SA - just ensure the service account exists
	logger := l.WithValues("User supplied moverServiceAccount", client.ObjectKeyFromObject(d.SA))

	err := d.Client.Get(ctx, client.ObjectKeyFromObject(d.SA), d.SA)
	if err != nil {
		logger.Error(err, "Unable to find user supplied moverServiceAccount")
		return nil, err
	}

	return d.SA, nil
}
