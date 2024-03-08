/*
Copyright © 2024 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

//nolint:dupl
package cmd

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationHandlerRsyncTLS struct{}

var _ replicationHandler = &replicationHandlerRsyncTLS{}

func (rhrtls *replicationHandlerRsyncTLS) ApplyDestination(ctx context.Context,
	c client.Client, dstPVC *corev1.PersistentVolumeClaim, addIDLabel func(obj client.Object),
	destConfig *replicationRelationshipDestinationV2) (*string, *corev1.Secret, error) {
	// Create destination
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destConfig.RDName,
			Namespace: destConfig.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, c, rd, func() error {
		addIDLabel(rd)
		rd.Spec = volsyncv1alpha1.ReplicationDestinationSpec{
			RsyncTLS: &volsyncv1alpha1.ReplicationDestinationRsyncTLSSpec{
				ReplicationDestinationVolumeOptions: destConfig.ReplicationDestinationVolumeOptions,
				ServiceType:                         destConfig.ServiceType,
			},
		}
		if dstPVC != nil {
			rd.Spec.RsyncTLS.DestinationPVC = &dstPVC.Name
		}
		return nil
	})
	if err != nil {
		klog.Errorf("unable to create ReplicationDestination: %v", err)
		return nil, nil, err
	}

	rd, err = rhrtls.awaitDestAddrKeys(ctx, c, client.ObjectKeyFromObject(rd))
	if err != nil {
		klog.Errorf("error while waiting for destination keys and address: %v", err)
		return nil, nil, err
	}

	// Fetch the keys
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *rd.Status.RsyncTLS.KeySecret,
			Namespace: destConfig.Namespace,
		},
	}
	if err = c.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		klog.Errorf("unable to retrieve tls keySecret: %v", err)
		return nil, nil, err
	}

	return rd.Status.RsyncTLS.Address, secret, nil
}

func (rhrtls *replicationHandlerRsyncTLS) awaitDestAddrKeys(ctx context.Context, c client.Client,
	rdName types.NamespacedName) (*volsyncv1alpha1.ReplicationDestination, error) {
	klog.Infof("waiting for keys & address of destination to be available")
	rd := volsyncv1alpha1.ReplicationDestination{}
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, defaultRsyncKeyTimeout, true, /*immediate*/
		func(ctx context.Context) (bool, error) {
			if err := c.Get(ctx, rdName, &rd); err != nil {
				return false, err
			}
			if rd.Status == nil || rd.Status.RsyncTLS == nil {
				return false, nil
			}
			if rd.Status.RsyncTLS.Address == nil {
				return false, nil
			}
			if rd.Status.RsyncTLS.KeySecret == nil {
				return false, nil
			}
			return true, nil
		})
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

func (rhrtls *replicationHandlerRsyncTLS) ApplySource(ctx context.Context, c client.Client,
	address *string, dstKeys *corev1.Secret, addIDLabel func(obj client.Object),
	sourceConfig *replicationRelationshipSourceV2) error {
	klog.Infof("creating resources on Source")
	srcKeys, err := rhrtls.applySourceKeys(ctx, c, dstKeys, addIDLabel, sourceConfig)
	if err != nil {
		klog.Errorf("unable to create source ssh keys: %v", err)
		return err
	}

	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceConfig.RSName,
			Namespace: sourceConfig.Namespace,
		},
	}
	_, err = ctrlutil.CreateOrUpdate(ctx, c, rs, func() error {
		addIDLabel(rs)
		rs.Spec = volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: sourceConfig.PVCName,
			Trigger:   &sourceConfig.Trigger,
			RsyncTLS: &volsyncv1alpha1.ReplicationSourceRsyncTLSSpec{
				ReplicationSourceVolumeOptions: sourceConfig.ReplicationSourceVolumeOptions,
			},
		}
		rs.Spec.RsyncTLS.Address = address
		rs.Spec.RsyncTLS.KeySecret = &srcKeys.Name
		return nil
	})
	return err
}

// Copies the ssh key secret into the source cluster
func (rhrtls *replicationHandlerRsyncTLS) applySourceKeys(ctx context.Context,
	c client.Client, dstKeys *corev1.Secret, addIDLabel func(obj client.Object),
	sourceConfig *replicationRelationshipSourceV2) (*corev1.Secret, error) {
	srcKeys := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceConfig.RSName,
			Namespace: sourceConfig.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, c, srcKeys, func() error {
		addIDLabel(srcKeys)
		srcKeys.Data = dstKeys.Data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return srcKeys, nil
}
