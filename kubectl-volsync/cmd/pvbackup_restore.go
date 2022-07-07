/*
Copyright © 2022 The VolSync authors

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
	"strconv"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pvBackupRestore struct {
	// Cluster context name
	Cluster string
	// Namespace on Source cluster
	Namespace string
	// PVC to which data to be restored
	destPVCName string
	// PVC object associated with pvcName used to create destination object
	destPVC persistentVolumeClaim
	// capacity is the size of the destination volume to create
	Capacity *resource.Quantity
	// AccessModes contains the desired access modes the volume should have
	AccessModes []corev1.PersistentVolumeAccessMode
	// Name of the restore
	backupInfo string
	// Name of the ReplicationDestination object
	RDName string
	// Name of the ReplicationSource object
	RSName string
	// Restore
	restoreAsOf string
	// specifies an offset for how many snapshots ago we want to restore from
	prev int32
	// restic configuration details
	resticConfig
	// read in restic-config into stringData
	stringData map[string]string
	// client object to communicate with a cluster
	client client.Client
	// backup relationship object to be persisted to a config file
	pr *pvBackupRelationship
}

// pvBackupRestoreCmd represents the create command
var pvBackupRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: i18n.T("Restore PV to latest/specified time stamp"),
	Long: templates.LongDesc(i18n.T(`This command creates the necessary configuration
	inside the Cluster/Namespace, builds the destination CR, restores the PC to
	latest/an upper-limit on the snapshots and saves the details into to relationship file.
	If --restoreAsOf flag is not provided, the data will be restored to latest snapshot.

	ex: # kubectl volsync pv-backup restore --pvcname dest/pvc1 --relationship pvr1
	--repository my-backup --restic-config restic-conf.toml --capacity 1Gi
	--restoreAsOf 2022-06-23T18:34:43+05:30

	NOTE: Example for restic-conf.toml can be found at
	"https://github.com/backube/volsync/tree/main/examples/restic/pv-backup/restic-conf.toml"`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		pc, err := newPVBackupRestore(cmd)
		if err != nil {
			return err
		}
		return pc.Run(cmd.Context())
	},
}

func init() {
	initPVBackupRestoreCmd(pvBackupRestoreCmd)
}

func initPVBackupRestoreCmd(pvBackupRestoreCmd *cobra.Command) {
	pvBackupCmd.AddCommand(pvBackupRestoreCmd)

	pvBackupRestoreCmd.Flags().String("accessmodes", "ReadWriteOnce",
		"accessMode of the PVC to create. viz: ReadWriteOnce, ReadWriteMany")
	pvBackupRestoreCmd.Flags().String("capacity", "", "size of the PVC to create (ex: 100Mi, 10Gi, 2Ti)")
	pvBackupRestoreCmd.Flags().String("copymethod", "Direct", `specifies how the data should be preserved
	at the end of each synchronization iteration`)
	pvBackupRestoreCmd.Flags().String("pvcname", "", "name of the PVC to backup: [context/]namespace/name")
	cobra.CheckErr(pvBackupRestoreCmd.MarkFlagRequired("pvcname"))
	pvBackupRestoreCmd.Flags().String("restic-config", "", `path for the restic config file`)
	cobra.CheckErr(pvBackupRestoreCmd.MarkFlagRequired("restic-config"))
	pvBackupRestoreCmd.Flags().String("previous", "", `Non-negative integer which
	specifies an offset for how many snapshots ago we want to restore from. When
	restoreAsOf is provided, the behavior is the same, however the starting snapshot
	considered will be the first one taken before restoreAsOf`)
	pvBackupRestoreCmd.Flags().String("repository", "", `This is the name of the Secret
	(in the same Namespace) that holds the connection information for the backup repository.
	The repository path should be unique for each PV.`)
	pvBackupRestoreCmd.Flags().String("restoreAsOf", "", `An RFC-3339 timestamp which specifies an upper-limit
	on the snapshots that we should be looking through when preparing to restore`)
}

func newPVBackupRestore(cmd *cobra.Command) (*pvBackupRestore, error) {
	pvr := &pvBackupRestore{}
	// build struct migrationRelationship from cmd line args
	pr, err := newPVBackupRelationship(cmd)
	if err != nil {
		return nil, err
	}
	pvr.pr = pr

	if err = pvr.parseCLI(cmd); err != nil {
		return nil, err
	}

	return pvr, nil
}

//nolint:funlen
func (pvr *pvBackupRestore) parseCLI(cmd *cobra.Command) error {
	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil || len(pvcname) == 0 {
		return fmt.Errorf("failed to fetch the pvcname, err = %w", err)
	}
	xcr, err := ParseXClusterName(pvcname)
	if err != nil {
		return fmt.Errorf("failed to parse cluster name from pvcname, err = %w", err)
	}
	pvr.destPVCName = xcr.Name
	pvr.Namespace = xcr.Namespace
	pvr.Cluster = xcr.Cluster

	sCapacity, err := cmd.Flags().GetString("capacity")
	if err != nil {
		return fmt.Errorf("failed to get capacity argument, err = %w", err)
	}
	if len(sCapacity) > 0 {
		capacity, err := resource.ParseQuantity(sCapacity)
		if err != nil {
			return fmt.Errorf("capacity must be a valid resource.Quantity: %w", err)
		}
		pvr.Capacity = &capacity
	}

	accessMode, err := cmd.Flags().GetString("accessmodes")
	if err != nil {
		return fmt.Errorf("failed to fetch access mode, %w", err)
	}

	if corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadWriteOnce &&
		corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadWriteMany {
		return fmt.Errorf("unsupported access mode: %v", accessMode)
	}
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(accessMode)}
	pvr.AccessModes = accessModes

	backupInfo, err := cmd.Flags().GetString("repository")
	if err != nil {
		return fmt.Errorf("failed to fetch the backup info, err = %w", err)
	}

	if len(backupInfo) == 0 {
		return fmt.Errorf("provide the backup info, err = %w", err)
	}
	pvr.backupInfo = backupInfo
	pvr.RDName = backupInfo + "-backup-destination"
	pvr.RSName = backupInfo + "-backup-source"

	resticConfigFile, err := cmd.Flags().GetString("restic-config")
	if err != nil {
		return fmt.Errorf("failed to fetch the restic-config, err = %w", err)
	}
	resticConfig, err := parseResticConfig(resticConfigFile)
	if err != nil {
		return err
	}
	pvr.resticConfig = *resticConfig

	stringData, err := parseSecretData(pvr.resticConfig.Viper)
	if err != nil {
		return err
	}

	pvr.stringData = stringData

	ts, err := cmd.Flags().GetString("restoreAsOf")
	if err != nil {
		return fmt.Errorf("failed to fetch the restoreAsOf flag, err = %w", err)
	}
	if len(ts) > 0 {
		pvr.restoreAsOf = ts
	}

	prev, err := cmd.Flags().GetString("previous")
	if err != nil {
		return fmt.Errorf("failed to fetch the previous flag, err = %w", err)
	}
	if len(prev) > 0 {
		prevInt, err := strconv.ParseInt(prev, 10, 32)
		if err != nil {
			return fmt.Errorf("string conversion resulted in error, %w", err)
		}
		pvr.prev = int32(prevInt)
	}

	return nil
}

//nolint:funlen
func (pvr *pvBackupRestore) Run(ctx context.Context) error {
	k8sClient, err := newClient(pvr.Cluster)
	if err != nil {
		return err
	}
	pvr.client = k8sClient

	// Ensure/create the Namespace to which user wants to restore the data
	_, err = pvr.ensureNamespace(ctx)
	if err != nil {
		return err
	}

	// Get the pvc from the cluster/namespace
	pvr.destPVC.pvc, err = pvr.getDestinationPVC(ctx)
	if err != nil {
		return err
	}

	// We need to make sure pvc is not use by anyother pod before restoring the data to avoid
	// data corruption
	if pvr.destPVC.pvc != nil {
		if err = pvr.destPVC.checkPVCMountStatus(ctx, k8sClient); err != nil {
			return err
		}
	}

	// Build struct pvBackupRelationshipSource from struct pvBackupRestore
	pvr.pr.data.Destination, err = pvr.newPVBackupRelationshipDestination()
	if err != nil {
		return err
	}

	// Creates the PVC if it doesn't exist
	_, err = pvr.ensureDestPVC(ctx)
	if err != nil {
		return err
	}

	// Add restic configurations into cluster
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvr.backupInfo + "-dest",
			Namespace: pvr.Namespace,
		},
		StringData: pvr.stringData,
	}
	err = createSecret(ctx, secret, pvr.client)
	if err != nil {
		return err
	}

	// Creates the RD if it doesn't exist
	_, err = pvr.ensureReplicationDestination(ctx)
	if err != nil {
		return err
	}

	// Wait for ReplicationDestination
	_, err = pvr.pr.data.Destination.waitForRDStatus(ctx, pvr.client)
	if err != nil {
		return err
	}

	// Save the replication destination details into relationship file
	if err = pvr.pr.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}

func (pvr *pvBackupRestore) newPVBackupRelationshipDestination() (*pvBackupRelationshipDestination, error) {
	if pvr.destPVC.pvc == nil {
		if pvr.Capacity == nil {
			return nil, fmt.Errorf("capacity arg must be provided")
		}
	}

	if len(pvr.restoreAsOf) == 0 {
		pvr.restoreAsOf = time.Now().Format(time.RFC3339)
	}
	// Assign the values from pvBackupRestore built after parsing cmd args
	return &pvBackupRelationshipDestination{
		Namespace: pvr.Namespace,
		RDName:    pvr.RDName,
		Trigger: volsyncv1alpha1.ReplicationDestinationTriggerSpec{
			Manual: time.Now().Format(time.RFC3339),
		},
		Destination: volsyncv1alpha1.ReplicationDestinationResticSpec{
			Repository: pvr.backupInfo + "-dest",
			ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
				CopyMethod:     volsyncv1alpha1.CopyMethodDirect,
				DestinationPVC: &pvr.destPVCName,
				Capacity:       pvr.Capacity,
			},
			RestoreAsOf: &pvr.restoreAsOf,
			Previous:    &pvr.prev,
		},
	}, nil
}

func (pvr *pvBackupRestore) ensureReplicationDestination(ctx context.Context) (
	*volsyncv1alpha1.ReplicationDestination, error) {
	prd := pvr.pr.data.Destination
	klog.Infof("Trigger restore As of %s", *prd.Destination.RestoreAsOf)
	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prd.RDName,
			Namespace: prd.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationDestinationSpec{
			Trigger: &prd.Trigger,
			Restic:  &prd.Destination,
		},
	}
	if err := pvr.client.Create(ctx, rd); err != nil {
		return nil, err
	}
	klog.Infof("Created ReplicationDestination: \"%s\" in Namespace: \"%s\" and Cluster: \"%s\"",
		rd.Name, rd.Namespace, pvr.Cluster)

	return rd, nil
}

func (prd *pvBackupRelationshipDestination) waitForRDStatus(ctx context.Context, client client.Client) (
	*volsyncv1alpha1.ReplicationDestination, error) {
	// wait for pvbackup destination CR to become ready
	var (
		rd  *volsyncv1alpha1.ReplicationDestination
		err error
	)
	klog.Infof("waiting for destination CR to be available")
	err = wait.PollImmediate(5*time.Second, defaultRsyncKeyTimeout, func() (bool, error) {
		rd, err = prd.getReplicationDestination(ctx, client)
		if err != nil {
			return false, err
		}
		if rd.Status == nil {
			return false, nil
		}

		klog.V(2).Infof("pvback Destination CR is up")
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rd status: %w,", err)
	}

	return rd, nil
}

func (pvr *pvBackupRestore) getDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	destPVC := &corev1.PersistentVolumeClaim{}
	pvcInfo := types.NamespacedName{
		Namespace: pvr.Namespace,
		Name:      pvr.destPVCName,
	}
	err := pvr.client.Get(ctx, pvcInfo, destPVC)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			klog.Infof("pvc: \"%s\" not found, creating the same", pvr.destPVCName)
			return nil, nil
		}
		return nil, err
	}
	return destPVC, nil
}

func (pvr *pvBackupRestore) ensureDestPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	if pvr.destPVC.pvc == nil {
		PVC, err := pvr.createDestinationPVC(ctx)
		if err != nil {
			return nil, err
		}
		pvr.destPVC.pvc = PVC
	} else {
		klog.Infof("Destination PVC: \"%s\" is found in Namespace: \"%s\" and is used to create replication destination",
			pvr.destPVC.pvc.Name, pvr.destPVC.pvc.Namespace)
	}

	return pvr.destPVC.pvc, nil
}

func (pvr *pvBackupRestore) createDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	destPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvr.destPVCName,
			Namespace: pvr.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      pvr.AccessModes,
			StorageClassName: nil,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *pvr.Capacity,
				},
			},
		},
	}

	if err := pvr.client.Create(ctx, destPVC); err != nil {
		return nil, err
	}

	klog.Infof("Created Destination PVC: \"%s\" in NameSpace: \"%s\" and Cluster: \"%s\" ",
		destPVC.Name, destPVC.Namespace, pvr.Cluster)

	return destPVC, nil
}

func (pvr *pvBackupRestore) ensureNamespace(ctx context.Context) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvr.Namespace,
		},
	}
	if err := pvr.client.Create(ctx, ns); err != nil {
		if kerrs.IsAlreadyExists(err) {
			klog.Infof("Namespace: \"%s\" is found, proceeding with the same",
				pvr.Namespace)
			return ns, nil
		}
		return nil, err
	}
	klog.Infof("Created Destination Namespace: \"%s\" in Cluster: \"%s\"", ns.Name, pvr.Cluster)

	return ns, nil
}
