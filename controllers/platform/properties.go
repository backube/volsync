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

	"github.com/backube/volsync/controllers/utils"
	"github.com/go-logr/logr"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Properties contains properties about the environment in which we are running
type Properties struct {
	IsOpenShift        bool // True if we are running on OpenShift
	HasSCCRestrictedV2 bool // True if the SecurityContextConstraints "restricted-v2" exists
}

//nolint:lll
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;patch;update

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

func EnsureVolSyncMoverSCCIfOpenShift(ctx context.Context, k8sClient client.Client, logger logr.Logger,
	sccName string, sccRaw []byte) error {
	openShift, err := isOpenShift(ctx, k8sClient, logger)
	if err != nil {
		return err
	}
	if !openShift {
		return nil // Not OpenShift, nothing to do here
	}

	l := logger.WithValues("scc name", sccName)

	decoder := serializer.NewCodecFactory(k8sClient.Scheme()).UniversalDeserializer()

	volsyncMoverScc := &ocpsecurityv1.SecurityContextConstraints{}
	_, _, err = decoder.Decode(sccRaw, nil, volsyncMoverScc)
	if err != nil {
		return err
	}
	// Set the name of the scc to match desired name
	volsyncMoverScc.Name = sccName

	// This is an OpenShift cluster - first determine if the SCC is already there
	// and if so, update it via a merge patch only if the SCC has label that indicates it was created by volsync
	currentScc := &ocpsecurityv1.SecurityContextConstraints{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: sccName}, currentScc)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// SCC doesn't exist, create it
			utils.SetOwnedByVolSync(volsyncMoverScc)

			l.Info("Creating volsync mover scc")
			return k8sClient.Create(ctx, volsyncMoverScc)
		}
		// Error retrieving SCC, return the err
		return err
	}

	// The scc is found on the system - to avoid issues with updating SCCs that perhaps we do not own,
	// only update the SCC if it has the owned by volsync label
	if !utils.IsOwnedByVolsync(currentScc) {
		l.Info("VolSync Mover SCC is not owned by VolSync, not updating.")
		return nil
	}

	// Update the scc via a merge patch
	l.Info("Updating volsync mover scc via merge patch")

	// Leave metadata untouched
	volsyncMoverScc.ObjectMeta = currentScc.ObjectMeta

	// Patch currentScc with our volsync mover scc
	return k8sClient.Patch(ctx, volsyncMoverScc, client.MergeFrom(currentScc))
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
