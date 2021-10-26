/*
Copyright © 2021 The VolSync authors

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
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSync struct {
	rel       *replicationRelationship
	srcClient client.Client
	dstClient client.Client
	srcPVC    *corev1.PersistentVolumeClaim
	srcSecret *corev1.Secret
}

// replicationSetDestinationCmd represents the replicationSetDestination command
var replicationSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: i18n.T("Replicate data to the destination once"),
	Long: templates.LongDesc(i18n.T(`
	This command replicates data to the destination a single time.

	This runs a manual sync from source to destination and waits for the sync to
	complete. It also removes any existing replication schedule.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		sync, err := newReplicationSync(cmd)
		if err != nil {
			return err
		}
		sync.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return sync.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationSyncCmd)
}

func newReplicationSync(cmd *cobra.Command) (*replicationSync, error) {
	sync := &replicationSync{}
	return sync, nil
}

// nolint: funlen
func (sync *replicationSync) Run(ctx context.Context) error {
	// Both Source and Destination need to be defined before using this command
	if sync.rel.data.Source == nil {
		return fmt.Errorf("a source must be defined before running 'sync' -- try 'set-source'")
	}
	if sync.rel.data.Destination == nil {
		return fmt.Errorf("a destination must be defined before running 'sync' -- try 'set-destination'")
	}

	var err error
	sync.srcClient, err = newClient(sync.rel.data.Source.Cluster)
	if err != nil {
		return fmt.Errorf("unable to create client for source cluster context: %w", err)
	}
	sync.dstClient, err = newClient(sync.rel.data.Destination.Cluster)
	if err != nil {
		return fmt.Errorf("unable to create client for destination cluster context: %w", err)
	}

	// Ensure destination
	rd, err := sync.createDestination(ctx)
	if err != nil {
		return fmt.Errorf("error creating ReplicationDestination: %w", err)
	}

	addr := rd.Status.Rsync.Address
	if err = sync.copyKeySecret(ctx, *rd.Status.Rsync.SSHKeys); err != nil {
		return fmt.Errorf("error copying ssh keys from destination to source: %w", err)
	}

	// Create the source
	sync.rel.data.Source.Source.Address = addr
	sync.rel.data.Source.Source.SSHKeys = &sync.srcSecret.Name
	rs, err := sync.createSource(ctx)
	if err != nil {
		return fmt.Errorf("error creating ReplicationSource: %w", err)
	}
	// Save the relationship since we now have a name for the RS and key secret
	if err = sync.rel.Save(); err != nil {
		return fmt.Errorf("error saving updated relationship: %w", err)
	}

	// Set ownership of srcKeys so they get cleaned up when RS is deleted
	err = sync.setSrcKeyOwnership(ctx, rs)
	if err != nil {
		return fmt.Errorf("error setting source key ownership: %w", err)
	}

	// Poll for completion
	err = wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		nsn := client.ObjectKeyFromObject(rs)
		err := sync.srcClient.Get(ctx, nsn, rs)
		if err != nil {
			return false, err
		}
		if rs.Status == nil ||
			rs.Status.LastManualSync != rs.Spec.Trigger.Manual {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for synchronization to complete: %w", err)
	}

	return nil
}

func (sync *replicationSync) createDestination(ctx context.Context) (*volsyncv1alpha1.ReplicationDestination, error) {
	// If CM:N, we're going to use destination PVC instead of letting the
	// operator create a temporary one. Either use the one that exists or create
	// one
	if sync.rel.data.Destination.Destination.CopyMethod == volsyncv1alpha1.CopyMethodNone {
		pvc, err := sync.createPVC(ctx)
		if err != nil {
			return nil, err
		}
		sync.rel.data.Destination.Destination.DestinationPVC = &pvc.Name
	} else {
		err := sync.getSrcPVC(ctx)
		if err != nil {
			return nil, err
		}

		rdSpec := &sync.rel.data.Destination.Destination
		if rdSpec.Capacity == nil {
			// Capacity hasn't been specified, so copy it from the source
			capacity := sync.srcPVC.Spec.Resources.Requests.Storage()
			statusCapacity := sync.srcPVC.Status.Capacity.Storage()
			if statusCapacity != nil {
				capacity = statusCapacity
			}
			rdSpec.Capacity = capacity
		}
		if len(rdSpec.AccessModes) == 0 {
			rdSpec.AccessModes = sync.srcPVC.Spec.AccessModes
		}
	}

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sync.rel.data.Destination.RDName,
			Namespace: sync.rel.data.Destination.Namespace,
		},
	}
	_, err := ctrlutil.CreateOrUpdate(ctx, sync.dstClient, rd, func() error {
		rd.Spec = volsyncv1alpha1.ReplicationDestinationSpec{
			Rsync: &sync.rel.data.Destination.Destination,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Wait for keys and address
	err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		nsn := client.ObjectKeyFromObject(rd)
		err := sync.dstClient.Get(ctx, nsn, rd)
		if err != nil {
			return false, err
		}
		if rd.Status == nil ||
			rd.Status.Rsync == nil ||
			rd.Status.Rsync.Address == nil ||
			rd.Status.Rsync.SSHKeys == nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return rd, nil
}

func (sync *replicationSync) createPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	// If PVC exists, return it
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			// We use the same name for the PVC and RD since it's only possible
			// for a PVC to be the destination of a single replication
			// relationship
			Name:      sync.rel.data.Destination.RDName,
			Namespace: sync.rel.data.Destination.Namespace,
		},
	}
	err := sync.dstClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
	if err == nil { // found it
		return pvc, nil
	} else if !kerrors.IsNotFound(err) { // some unexpected error
		return nil, err
	}

	// Get capacity of source PVC
	err = sync.getSrcPVC(ctx)
	if err != nil {
		return nil, err
	}

	// Create PVC using info from the RD or fall back to copying from the Source's PVC
	pvc.Spec.AccessModes = sync.rel.data.Destination.Destination.AccessModes
	if len(pvc.Spec.AccessModes) == 0 {
		pvc.Spec.AccessModes = sync.srcPVC.Spec.AccessModes
	}
	if sync.rel.data.Destination.Destination.Capacity != nil {
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *sync.rel.data.Destination.Destination.Capacity
	} else {
		capacity := sync.srcPVC.Spec.Resources.Requests.Storage()
		statusCapacity := sync.srcPVC.Status.Capacity.Storage()
		if statusCapacity != nil {
			capacity = statusCapacity
		}
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *capacity
	}
	err = sync.dstClient.Create(ctx, pvc)
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

func (sync *replicationSync) getSrcPVC(ctx context.Context) error {
	sync.srcPVC = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sync.rel.data.Source.PVCName,
			Namespace: sync.rel.data.Source.Namespace,
		},
	}
	return sync.srcClient.Get(ctx, client.ObjectKeyFromObject(sync.srcPVC), sync.srcPVC)
}

func (sync *replicationSync) copyKeySecret(ctx context.Context, dstKeyName string) error {
	dstSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dstKeyName,
			Namespace: sync.rel.data.Destination.Namespace,
		},
	}
	err := sync.dstClient.Get(ctx, client.ObjectKeyFromObject(dstSecret), dstSecret)
	if err != nil {
		return err
	}

	// Update source secret if it's already defined
	if len(sync.rel.data.Source.SSHKeyName) > 0 {
		sync.srcSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sync.rel.data.Source.SSHKeyName,
				Namespace: sync.rel.data.Source.Namespace,
			},
		}
		_, err := ctrlutil.CreateOrUpdate(ctx, sync.srcClient, sync.srcSecret, func() error {
			sync.srcSecret.Data = dstSecret.Data
			return nil
		})
		return err
	}

	// Create source secret since it's new
	sync.srcSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: sync.rel.data.Source.PVCName + "-",
			Namespace:    sync.rel.data.Source.Namespace,
		},
		Data: dstSecret.Data,
	}

	err = sync.srcClient.Create(ctx, sync.srcSecret)
	sync.rel.data.Source.SSHKeyName = sync.srcSecret.Name
	return err
}

func (sync *replicationSync) createSource(ctx context.Context) (*volsyncv1alpha1.ReplicationSource, error) {
	syncString := time.Now().Format(time.RFC3339)

	if len(sync.rel.data.Source.RSName) > 0 {
		// We believe an RS should already exist, so we'll update it
		rs := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sync.rel.data.Source.RSName,
				Namespace: sync.rel.data.Source.Namespace,
			},
		}
		_, err := ctrlutil.CreateOrUpdate(ctx, sync.srcClient, rs, func() error {
			rs.Spec = volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: sync.rel.data.Source.PVCName,
				Trigger: &volsyncv1alpha1.ReplicationSourceTriggerSpec{
					Manual: syncString,
				},
				Rsync: &sync.rel.data.Source.Source,
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return rs, nil
	}

	// Create a new replication source
	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: sync.rel.data.Source.PVCName + "-",
			Namespace:    sync.rel.data.Source.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: sync.rel.data.Source.PVCName,
			Trigger: &volsyncv1alpha1.ReplicationSourceTriggerSpec{
				Manual: syncString,
			},
			Rsync: &sync.rel.data.Source.Source,
		},
	}

	err := sync.srcClient.Create(ctx, rs)
	if err != nil {
		return nil, err
	}
	sync.rel.data.Source.RSName = rs.Name
	return rs, nil
}

func (sync *replicationSync) setSrcKeyOwnership(ctx context.Context, owner metav1.Object) error {
	err := ctrlutil.SetOwnerReference(owner, sync.srcSecret, sync.srcClient.Scheme())
	if err != nil {
		return err
	}
	return sync.srcClient.Update(ctx, sync.srcSecret)
}
