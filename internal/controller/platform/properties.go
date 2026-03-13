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
	ocpconfigv1 "github.com/openshift/api/config/v1"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/backube/volsync/internal/controller/utils"
)

// Properties contains properties about the environment in which we are running
var properties *Properties

type Properties struct {
	IsOpenShift            bool                        // True if we are running on OpenShift
	TLSSecurityProfileSpec *ocpconfigv1.TLSProfileSpec // Will be nil if not on OpenShift
}

//nolint:lll
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;patch;update
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch

// Retrieves properties of the running cluster
func GetProperties(ctx context.Context, k8sClient client.Client, logger logr.Logger) (Properties, error) {
	if properties != nil {
		// Use cached value if it's set
		return *properties, nil
	}

	if err := ocpsecurityv1.AddToScheme(k8sClient.Scheme()); err != nil {
		logger.Error(err, "unable to add scheme for security.openshift.io")
		return Properties{}, err
	}
	if err := ocpconfigv1.AddToScheme(k8sClient.Scheme()); err != nil {
		logger.Error(err, "unable to add scheme for config.openshift.io")
		return Properties{}, err
	}

	var err error
	p := Properties{}

	if p.IsOpenShift, err = isOpenShift(ctx, k8sClient, logger); err != nil {
		return Properties{}, err
	}
	if p.IsOpenShift {
		if p.TLSSecurityProfileSpec, err = getTLSProfile(ctx, k8sClient, logger); err != nil {
			return Properties{}, err
		}
	}

	// Cache properties for subsequent calls
	properties = &p

	return p, nil
}

// For test usage, clear out our cached properties
func clearProperties() {
	properties = nil
}

// Checks to determine whether this is OpenShift by looking for any SecurityContextConstraint objects
func isOpenShift(ctx context.Context, k8sClient client.Client, logger logr.Logger) (bool, error) {
	SCCs := ocpsecurityv1.SecurityContextConstraintsList{}
	err := k8sClient.List(ctx, &SCCs)
	if len(SCCs.Items) > 0 {
		return true, nil
	}
	if err == nil || utils.IsCRDNotPresentError(err) {
		return false, nil
	}
	logger.Error(err, "error while looking for SCCs")
	return false, err
}

func EnsureVolSyncMoverSCCIfOpenShift(ctx context.Context, k8sClient client.Client, logger logr.Logger,
	sccName string, sccRaw []byte) error {
	p, err := GetProperties(ctx, k8sClient, logger)
	if err != nil {
		return err
	}
	if !p.IsOpenShift {
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
