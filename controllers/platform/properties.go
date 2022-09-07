/*
Copyright 2022 The VolSync authors.

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

package platform

import (
	"context"

	"github.com/go-logr/logr"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Properties contains properties about the environment in which we are running
type Properties struct {
	IsOpenShift        bool // True if we are running on OpenShift
	HasSCCRestrictedV2 bool // True if the SecurityContextConstraints "restricted-v2" exists
}

//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch

// Retrieves properties of the running cluster
func GetProperties(ctx context.Context, client client.Client, logger logr.Logger) (Properties, error) {
	if err := ocpsecurityv1.AddToScheme(client.Scheme()); err != nil {
		logger.Error(err, "unable to add scheme for security.openshift.io")
		return Properties{}, err
	}

	var err error
	p := Properties{}

	if p.IsOpenShift, err = isOpenShift(ctx, client, logger); err != nil {
		return Properties{}, err
	}
	if p.IsOpenShift {
		if p.HasSCCRestrictedV2, err = hasSCCRestrictedV2(ctx, client, logger); err != nil {
			return Properties{}, err
		}
	}
	return p, nil
}

// Checks to determine whether this is OpenShift by looking for any SecurityContextConstraint objects
func isOpenShift(ctx context.Context, c client.Client, l logr.Logger) (bool, error) {
	SCCs := ocpsecurityv1.SecurityContextConstraintsList{}
	err := c.List(ctx, &SCCs)
	if len(SCCs.Items) > 0 {
		return true, nil
	}
	if err == nil || apimeta.IsNoMatchError(err) || kerrors.IsNotFound(err) {
		return false, nil
	}
	l.Error(err, "error while looking for SCCs")
	return false, err
}

func hasSCCRestrictedV2(ctx context.Context, c client.Client, l logr.Logger) (bool, error) {
	scc := ocpsecurityv1.SecurityContextConstraints{
		ObjectMeta: metav1.ObjectMeta{
			Name: "restricted-v2",
		},
	}
	// The following assumes SCC is a valid type (i.e., it's OpenShift)
	err := c.Get(ctx, client.ObjectKeyFromObject(&scc), &scc)
	if err == nil {
		return true, nil
	}
	if kerrors.IsNotFound(err) {
		return false, nil
	}
	l.Error(err, "error while looking for restricted-v2 SCC")
	return false, err
}
