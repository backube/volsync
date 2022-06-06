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
const DefaultSCCName = "volsync-mover"

// SCCName is the name of the SCC to use for the mover Jobs
var SCCName string

type SAHandler struct {
	Context     context.Context
	Client      client.Client
	SA          *corev1.ServiceAccount
	Owner       metav1.Object
	role        *rbacv1.Role
	roleBinding *rbacv1.RoleBinding
}

func NewSAHandler(ctx context.Context, c client.Client, owner metav1.Object, sa *corev1.ServiceAccount) SAHandler {
	return SAHandler{
		Context: ctx,
		Client:  c,
		SA:      sa,
		Owner:   owner,
	}
}

func (d *SAHandler) Reconcile(l logr.Logger) (bool, error) {
	return ReconcileBatch(l,
		d.ensureSA,
		d.ensureRole,
		d.ensureRoleBinding,
	)
}

func (d *SAHandler) ensureSA(l logr.Logger) (bool, error) {
	logger := l.WithValues("ServiceAccount", client.ObjectKeyFromObject(d.SA))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.SA, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.SA, d.Client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		SetOwnedByVolSync(d.Owner, d.SA)
		return nil
	})
	if err != nil {
		logger.Error(err, "ServiceAccount reconcile failed")
		return false, err
	}

	logger.V(1).Info("ServiceAccount reconciled", "operation", op)
	return true, nil
}

func (d *SAHandler) ensureRole(l logr.Logger) (bool, error) {
	d.role = &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.SA.Name,
			Namespace: d.SA.Namespace,
		},
	}
	logger := l.WithValues("Role", client.ObjectKeyFromObject(d.role))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.role, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.role, d.Client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		SetOwnedByVolSync(d.Owner, d.role)
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
		return nil
	})
	if err != nil {
		logger.Error(err, "Role reconcile failed")
		return false, err
	}

	logger.V(1).Info("Role reconciled", "operation", op)
	return true, nil
}

func (d *SAHandler) ensureRoleBinding(l logr.Logger) (bool, error) {
	d.roleBinding = &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.SA.Name,
			Namespace: d.SA.Namespace,
		},
	}
	logger := l.WithValues("RoleBinding", client.ObjectKeyFromObject(d.roleBinding))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.roleBinding, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.roleBinding, d.Client.Scheme()); err != nil {
			logger.Error(err, "unable to set controller reference")
			return err
		}
		SetOwnedByVolSync(d.Owner, d.roleBinding)
		d.roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     d.role.Name,
		}
		d.roleBinding.Subjects = []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: d.SA.Name},
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
