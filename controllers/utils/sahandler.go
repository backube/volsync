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
	"strings"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DefaultSCCName is the default name of the volsync security context constraint
const DefaultSCCName = "volsync-privileged-mover" // #nosec G101 - gosec thinks this is a credential

// SCCName is the name of the SCC to use for the mover Jobs
var SCCName string

// Comma separated list of secrets to be copied from the volsync install namespace (typically volsync-system)
// to the mover's namespace and use for the mover service account - set via cmd line flag in main.go
var MoverImagePullSecrets string

// Returns map with keys being the img pull secret names from the volsync controller namespace
// and values being the names we should give these secrets in the mover namespace
var getMoverImagePullSecretsAsMap = sync.OnceValue(func() map[string]string {
	return ParseMoverImagePullSecrets(MoverImagePullSecrets)
})

func ParseMoverImagePullSecrets(moverImagePullSecrets string) map[string]string {
	pullSecretsMap := map[string]string{}
	origSecretNames := strings.Split(moverImagePullSecrets, ",")
	for _, orig := range origSecretNames {
		if orig != "" {
			sMoverCopy := "volsync-pull-" + GetHashedName(orig)

			pullSecretsMap[orig] = sMoverCopy
		}
	}
	return pullSecretsMap
}

type SAHandler interface {
	Reconcile(ctx context.Context, l logr.Logger) (*corev1.ServiceAccount, error)
}

type SAHandlerVolSync struct {
	Context          context.Context
	Client           client.Client
	SA               *corev1.ServiceAccount
	Owner            metav1.Object
	Privileged       bool
	role             *rbacv1.Role
	roleBinding      *rbacv1.RoleBinding
	PullSecretsMap   map[string]string
	VolSyncNamespace string
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
			Client:           c,
			SA:               sa,
			Owner:            owner,
			Privileged:       privileged,
			PullSecretsMap:   getMoverImagePullSecretsAsMap(),
			VolSyncNamespace: getVolSyncNamespace(),
		}
	}

	// User has supplied a moverServiceAccount - use SAHandlerUserSupplied to ensure it exists
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
		d.ensureMoverImagePullSecrets,
		d.ensureSA,
		d.ensureRole,
		d.ensureRoleBinding,
	)
	if cont {
		return d.SA, err
	}
	return nil, err
}

// Copy the list of image pull secrets from the volsync namespace to the mover's namespace
func (d *SAHandlerVolSync) ensureMoverImagePullSecrets(l logr.Logger) (bool, error) {
	for orig, moverCopy := range d.PullSecretsMap {
		err := d.ensureSecretCopyInMoverNamespace(l, orig, moverCopy)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// reconciles a copy of the original pull secret - copied from volsync controller namespace to the mover ns
// to avoid collisions, it will also be renamed
func (d *SAHandlerVolSync) ensureSecretCopyInMoverNamespace(l logr.Logger,
	origSecretName, copySecretName string) error {
	origSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      origSecretName,
			Namespace: d.VolSyncNamespace,
		},
	}

	err := d.Client.Get(d.Context, client.ObjectKeyFromObject(origSecret), origSecret)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}

		// Not throwing an error in case the original pull secret(s) do not exist - printing warning for now so we can
		// continue even if pull secrets aren't there
		l.Info("Warning, unable to find pull secret in the volsync controller namespace",
			"secret", origSecret.GetName(), "secret namespace", origSecret.GetNamespace())
		return nil
	}

	logger := l.WithValues("Orig pull secret", origSecretName, "ns", d.VolSyncNamespace,
		"mover copy pull secret", copySecretName, "ns", d.SA.GetNamespace())

	copySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      copySecretName,
			Namespace: d.SA.GetNamespace(),
		},
	}

	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, copySecret, func() error {
		// Add owner reference as our owning CR
		// This way the last owning CR to get deleted should also clean up this secret
		// via garbage collection
		err := ctrlutil.SetOwnerReference(d.Owner, copySecret, d.Client.Scheme())
		if err != nil {
			return err
		}

		if copySecret.CreationTimestamp.IsZero() { // Set only on create
			copySecret.Type = origSecret.Type
		}

		// Label the secret to indicate this was created by volsync
		SetOwnedByVolSync(copySecret)

		copySecret.Data = origSecret.Data

		return nil
	})
	if err != nil {
		logger.Error(err, "Mover pull secret reconcile failed")
		return err
	}

	logger.V(1).Info("Mover pull secret reconciled", "operation", op)
	return nil
}

func (d *SAHandlerVolSync) ensureSA(l logr.Logger) (bool, error) {
	logger := l.WithValues("ServiceAccount", client.ObjectKeyFromObject(d.SA))
	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.SA, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.SA, d.Client.Scheme()); err != nil {
			logger.Error(err, ErrUnableToSetControllerRef)
			return err
		}
		SetOwnedByVolSync(d.SA)

		// If there are any mover pull secrets, make sure they're added to the svc account img pull secrets
		for _, moverPullSecret := range d.PullSecretsMap {
			d.SA.ImagePullSecrets = addImgPullSec(d.SA.ImagePullSecrets, moverPullSecret)
		}

		return nil
	})
	if err != nil {
		logger.Error(err, "ServiceAccount reconcile failed")
		return false, err
	}

	logger.V(1).Info("ServiceAccount reconciled", "operation", op)
	return true, nil
}

func addImgPullSec(imagePullSecrets []corev1.LocalObjectReference, secretName string) []corev1.LocalObjectReference {
	for _, ips := range imagePullSecrets {
		if ips.Name == secretName {
			return imagePullSecrets // Nothing to update
		}
	}
	// image pull secrets slice does not contain the secret, so add it
	return append(imagePullSecrets, corev1.LocalObjectReference{Name: secretName})
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
