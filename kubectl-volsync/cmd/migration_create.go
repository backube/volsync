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

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DestinationInfo struct {
	Name         string
	Namespace    string
	Opts         VolSyncOptions
	CopyMethod   string
	Capacity     string
	StorageClass string
	AccessMode   string
	PVC          string
	ServiceType  string
}

type VolSyncOptions struct {
	KubeContext     string
	KubeClusterName string
	Namespace       string
	Client          client.Client
	CopyMethod      volsyncv1alpha1.CopyMethodType
	Capacity        resource.Quantity
	StorageClass    *string
	SSHUser         *string
	ServiceType     corev1.ServiceType
}

type SourceInfo struct {
}

type MigrationParams struct {
	Dest   DestinationInfo
	Source SourceInfo

	genericclioptions.IOStreams
}

// migrationCreateCmd represents the create command
var migrationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: i18n.T("Create a new migration destination"),
	Long: templates.LongDesc(i18n.T(`
	This command creates and prepares new migration destination to receive data.

	It creates the named PersistentVolumeClaim if it does not already exist,
	and it sets up an associated ReplicationDestination that will be configured
	to accept incoming transfers via rsync over ssh.
	`)),
	RunE: doMigrationCreate,
	Args: validateMigrationCreate,
}

func init() {
	migrationCmd.AddCommand(migrationCreateCmd)

	migrationCreateCmd.Flags().String("accessmodes", "", "AccessModes of the PVC to create")
	migrationCreateCmd.Flags().String("capacity", "", "capacity of the PVC to create")
	migrationCreateCmd.Flags().String("pvcname", "", "name of the PVC to create or use: [context/]namespace/name")
	cobra.CheckErr(migrationCreateCmd.MarkFlagRequired("pvcname"))
	migrationCreateCmd.Flags().String("storageclass", "", "StorageClass name for the PVC")
	migrationCreateCmd.Flags().String("servicetype", "", "ServiceType for the cluster, ex: ClusterIP, LoadBalancer")
}

func validateMigrationCreate(cmd *cobra.Command, args []string) error {
	// If specified, the PVC's capacity must parse to a valid resource.Quantity
	capacity, err := cmd.Flags().GetString("capacity")
	if err != nil {
		return err
	}
	if len(capacity) > 0 {
		if _, err := resource.ParseQuantity(capacity); err != nil {
			return fmt.Errorf("capacity must be a valid resource.Quantity: %w", err)
		}
	}
	// The PVC name must be specified, and it needs to be in the right format
	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil {
		return err
	}
	if _, err := ParseXClusterName(pvcname); err != nil {
		return err
	}
	return nil
}

//nolint:funlen
func doMigrationCreate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	configDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return err
	}
	rName, err := cmd.Flags().GetString("relationship")
	if err != nil {
		return err
	}
	// Create the empty relationship
	relation, err := CreateRelationship(configDir, rName, MigrationRelationship)
	if err != nil {
		return err
	}
	// Prepare the MigrationParams struct from cmd args
	migParams, err := prepMigrationParamsStruct(cmd, relation)
	if err != nil {
		return fmt.Errorf("failed to build migration params structure from the cmd flags %w", err)
	}
	// Check the namespace, if not present already then create the same
	err = checkAndCreateNamespace(ctx, migParams)
	if err != nil {
		return err
	}

	// Check create dest pvc if the pvc already present
	_, err = GetDestinationPVC(ctx, migParams)
	if err != nil {
		return err
	}

	err = CreateDestination(ctx, migParams)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func CreateDestination(ctx context.Context, migParams *MigrationParams) error {
	rsyncSpec := &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
		ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
			CopyMethod:     volsyncv1alpha1.CopyMethodNone,
			DestinationPVC: &migParams.Dest.PVC,
		},
		ServiceType: &migParams.Dest.Opts.ServiceType,
	}

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migParams.Dest.Name,
			Namespace: migParams.Dest.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationDestinationSpec{
			Rsync: rsyncSpec,
		},
	}
	klog.V(2).Infof("Creating ReplicationDestination in namespace %s", migParams.Dest.Namespace)
	if err := migParams.Dest.Opts.Client.Create(ctx, rd); err != nil {
		return err
	}
	klog.V(0).Infof("ReplicationDestination created in namespace %s", migParams.Dest.Namespace)

	return nil
}

func prepMigrationParamsStruct(cmd *cobra.Command, relation *Relationship) (*MigrationParams, error) {
	// Insert information into the relationship & save it
	cap, err := cmd.Flags().GetString("capacity")
	if err != nil {
		return nil, err
	}
	relation.Set("capacity", cap)

	pvcName, err := cmd.Flags().GetString("pvcname")
	if err != nil {
		return nil, err
	}
	relation.Set("pvcname", pvcName)

	x, err := ParseXClusterName(pvcName)
	if err != nil {
		return nil, err
	}
	nsName := x.Namespace
	pvc := x.Name

	accessMode, err := cmd.Flags().GetString("accessmodes")
	if err != nil {
		return nil, err
	}
	relation.Set("accessmodes", accessMode)

	serviceType, err := cmd.Flags().GetString("servicetype")
	if err != nil {
		return nil, err
	}
	relation.Set("serviceType", serviceType)
	if err = relation.Save(); err != nil {
		return nil, fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	// TODO: Set up objects on the cluster
	clientObject, _ := newClient(x.Cluster)

	return &MigrationParams{
		Dest: DestinationInfo{
			//Replication Destination name can be pvc name
			Name:        pvc,
			Namespace:   nsName,
			PVC:         pvc,
			Capacity:    cap,
			ServiceType: serviceType,
			Opts: VolSyncOptions{
				KubeClusterName: x.Cluster,
				Client:          clientObject,
			},
		},
	}, nil
}

//nolint:funlen
func GetDestinationPVC(ctx context.Context, migParams *MigrationParams) (*corev1.PersistentVolumeClaim, error) {
	destPVC := &corev1.PersistentVolumeClaim{}
	pvcInfo := types.NamespacedName{
		Namespace: migParams.Dest.Namespace,
		Name:      migParams.Dest.PVC,
	}

	err := migParams.Dest.Opts.Client.Get(ctx, pvcInfo, destPVC)
	if err == nil {
		return destPVC, nil
	}

	/* Create PVC is not pre-exists */
	if len(migParams.Dest.Capacity) == 0 {
		return nil, fmt.Errorf("%w, please provide the storage capacity to create destination pvc", err)
	}

	capacity, _ := resource.ParseQuantity(migParams.Dest.Capacity)
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	destPVC = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migParams.Dest.PVC,
			Namespace: migParams.Dest.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: capacity,
				},
			},
		},
	}

	if err := migParams.Dest.Opts.Client.Create(ctx, destPVC); err != nil {
		return nil, err
	}

	return destPVC, nil
}

func checkAndCreateNamespace(ctx context.Context, migParams *MigrationParams) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: migParams.Dest.Namespace,
		},
	}

	klog.V(2).Infof("Creating NameSpace %s ", migParams.Dest.Namespace)
	if err := migParams.Dest.Opts.Client.Create(ctx, ns); err != nil {
		if kerrs.IsAlreadyExists(err) {
			klog.V(2).Info("Namespace: %v already present, proceeding with this namespace", migParams.Dest.Namespace)
			return nil
		}
		return err
	}
	klog.Infof("Created destination namespace: %s", migParams.Dest.Namespace)
	return nil
}
