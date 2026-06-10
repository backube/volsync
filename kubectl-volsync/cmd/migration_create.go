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

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type migrationCreate struct {
	// migration relationship object to be persisted to a config file
	mr *migrationRelationship
	// Cluster context name
	Cluster string
	// Namespace on destination cluster
	Namespace string
	// destinationPVC is a PVC to use as the transfer destination instead of
	// automatically provisioning one. Either this field or both capacity and
	// accessModes must be specified.
	DestinationPVC string
	// Name of the ReplicationDestination object
	RDName string
	// True if the ReplicationDestination should use RsyncTLS
	IsRsyncTLS bool
	// copyMethod describes how a point-in-time (PiT) image of the destination
	// volume should be created
	CopyMethod volsyncv1alpha1.CopyMethodType
	// capacity is the size of the destination volume to create
	Capacity *resource.Quantity
	// storageClassName can be used to specify the StorageClass of the
	// destination volume. If not set, the default StorageClass will be used.
	StorageClassName *string
	// AccessModes contains the desired access modes the volume should have
	AccessModes []corev1.PersistentVolumeAccessMode
	// serviceType determines the Service type that will be created for incoming
	// SSH connections.
	ServiceType *corev1.ServiceType
	// client object to communicate with a cluster
	client client.Client
	// PVC object associated with pvcName used to create destination object
	PVC *corev1.PersistentVolumeClaim
	// MoverSecurity context to use for the ReplicationDestination
	// Individual fields will come from cli parameters
	MoverSecurityContext *corev1.PodSecurityContext
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		mc, err := newMigrationCreate(cmd)
		if err != nil {
			return err
		}
		return mc.Run(cmd.Context())
	},
}

func init() {
	initMigrationCreateCmd(migrationCreateCmd)
}

func initMigrationCreateCmd(migrationCreateCmd *cobra.Command) {
	migrationCmd.AddCommand(migrationCreateCmd)

	migrationCreateCmd.Flags().String("accessmodes", "ReadWriteOnce",
		"accessMode of the PVC to create. viz: ReadWriteOnce, ReadOnlyMany, ReadWriteMany, ReadWriteOncePod")
	migrationCreateCmd.Flags().String("capacity", "", "size of the PVC to create (ex: 100Mi, 10Gi, 2Ti)")
	migrationCreateCmd.Flags().String("pvcname", "", "name of the PVC to create or use: [context/]namespace/name")
	cobra.CheckErr(migrationCreateCmd.MarkFlagRequired("pvcname"))
	migrationCreateCmd.Flags().String("storageclass", "", "StorageClass name for the PVC")
	migrationCreateCmd.Flags().String("servicetype", "LoadBalancer",
		"Service Type or ingress methods for a service. viz: ClusterIP, LoadBalancer")
	migrationCreateCmd.Flags().String("rdname", "", "name of the ReplicationDestination to create")
	migrationCreateCmd.Flags().Bool("rsynctls", false, "if true, will use rsync-tls")

	// MoverSecurityContext flags - will only apply if rsyncTLS is true
	addCLIRsyncTLSMoverSecurityContextFlags(migrationCreateCmd, true)
}

func newMigrationCreate(cmd *cobra.Command) (*migrationCreate, error) {
	mc := &migrationCreate{}
	// build struct migrationRelationship from cmd line args
	mr, err := newMigrationRelationship(cmd)
	if err != nil {
		return nil, err
	}
	mc.mr = mr

	if err = mc.parseCLI(cmd); err != nil {
		return nil, err
	}

	mc.mr.data.IsRsyncTLS = mc.IsRsyncTLS
	if mc.IsRsyncTLS {
		mc.mr.mh = &migrationHandlerRsyncTLS{}
	} else {
		mc.mr.mh = &migrationHandlerRsync{}
	}

	return mc, nil
}

//nolint:funlen
func (mc *migrationCreate) parseCLI(cmd *cobra.Command) error {
	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil || pvcname == "" {
		return fmt.Errorf("failed to fetch the pvcname, err = %w", err)
	}
	xcr, err := ParseXClusterName(pvcname)
	if err != nil {
		return fmt.Errorf("failed to parse cluster name from pvcname, err = %w", err)
	}
	mc.DestinationPVC = xcr.Name
	mc.Namespace = xcr.Namespace
	mc.Cluster = xcr.Cluster

	sCapacity, err := cmd.Flags().GetString("capacity")
	if err != nil {
		return err
	}
	if len(sCapacity) > 0 {
		capacity, err := resource.ParseQuantity(sCapacity)
		if err != nil {
			return fmt.Errorf("capacity must be a valid resource.Quantity: %w", err)
		}
		mc.Capacity = &capacity
	}

	accessMode, err := cmd.Flags().GetString("accessmodes")
	if err != nil {
		return fmt.Errorf("failed to fetch access mode, %w", err)
	}

	if corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadWriteOnce &&
		corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadOnlyMany &&
		corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadWriteMany &&
		corev1.PersistentVolumeAccessMode(accessMode) != corev1.ReadWriteOncePod {
		return fmt.Errorf("unsupported access mode: %v", accessMode)
	}
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(accessMode)}
	mc.AccessModes = accessModes

	storageClass, err := cmd.Flags().GetString("storageclass")
	if err != nil {
		return fmt.Errorf("failed to fetch storageClass, %w", err)
	}
	if storageClass == "" {
		mc.StorageClassName = nil
	} else {
		mc.StorageClassName = &storageClass
	}
	// For migration use case, only Copymethod="Direct" is supported
	mc.CopyMethod = volsyncv1alpha1.CopyMethodDirect

	serviceType, err := cmd.Flags().GetString("servicetype")
	if err != nil {
		return fmt.Errorf("please provide service type, err = %w", err)
	}

	if corev1.ServiceType(serviceType) != corev1.ServiceTypeClusterIP &&
		corev1.ServiceType(serviceType) != corev1.ServiceTypeLoadBalancer {
		return fmt.Errorf("unsupported service type: %v", corev1.ServiceType(serviceType))
	}
	mc.ServiceType = (*corev1.ServiceType)(&serviceType)

	rdName, err := cmd.Flags().GetString("rdname")
	if err != nil {
		return fmt.Errorf("failed to fetch rdname, %w", err)
	}
	if rdName == "" {
		rdName = mc.DestinationPVC + "-mig-dst" // Generate default value
	}
	mc.RDName = rdName

	isRsyncTLS, err := cmd.Flags().GetBool("rsynctls")
	if err != nil {
		return fmt.Errorf("failed to fetch rsynctls, %w", err)
	}
	mc.IsRsyncTLS = isRsyncTLS

	if isRsyncTLS {
		// Parse the moverSecurityContext flags (these flags will not apply to the
		// rsync ssh case)
		mc.MoverSecurityContext, err = parseCLIRsyncTLSMoverSecurityContextFlags(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:funlen
func (mc *migrationCreate) newMigrationRelationshipDestination() (
	*migrationRelationshipDestinationV2, error) {
	mrd := &migrationRelationshipDestinationV2{}

	// Assign the values from migrationCreate built after parsing cmd args
	mrd.RDName = mc.RDName
	mrd.PVCName = mc.DestinationPVC
	mrd.Namespace = mc.Namespace
	mrd.Cluster = mc.Cluster

	if mc.PVC == nil {
		if mc.Capacity == nil {
			return nil, fmt.Errorf("capacity arg must be provided")
		}
	}

	// Some migration create Cli parameters such as capacity, storageclassname, accesmodes etc are not
	// saved to the migrationrelationship .yaml file.  These are only used at create time when the
	// destination PVC and ReplicationDestination will be created.

	mrd.ServiceType = mc.ServiceType
	mrd.CopyMethod = mc.CopyMethod

	return mrd, nil
}

func (mc *migrationCreate) Run(ctx context.Context) error {
	k8sClient, err := newClient(mc.Cluster)
	if err != nil {
		return err
	}
	mc.client = k8sClient

	// Get the pvc from cluster
	mc.PVC, err = mc.getDestinationPVC(ctx)
	if err != nil {
		return err
	}

	// Build struct migrationRelationshipDestination from struct migrationCreate
	mc.mr.data.Destination, err = mc.newMigrationRelationshipDestination()
	if err != nil {
		return err
	}
	// Creates the Namespace if it doesn't exist
	_, err = mc.ensureNamespace(ctx)
	if err != nil {
		return err
	}
	// Creates the PVC if it doesn't exist
	_, err = mc.ensureDestPVC(ctx)
	if err != nil {
		return err
	}

	// Creates the RD if it doesn't exist
	rd, err := mc.mr.mh.EnsureReplicationDestination(ctx, mc.client, mc.mr.data.Destination)
	//_, err = mc.ensureReplicationDestination(ctx)
	if err != nil {
		return err
	}

	// Wait for ReplicationDestination to post address, sshkeys
	_, err = mc.mr.mh.WaitForRDStatus(ctx, mc.client, rd)
	if err != nil {
		return err
	}
	// Save the destination details into relationship file
	if err = mc.mr.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}
	return nil
}

func (mc *migrationCreate) ensureNamespace(ctx context.Context) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mc.Namespace,
		},
	}
	if err := mc.client.Create(ctx, ns); err != nil {
		if kerrs.IsAlreadyExists(err) {
			klog.Infof("Namespace: \"%s\" is found, proceeding with the same",
				mc.Namespace)
			return ns, nil
		}
		return nil, err
	}
	klog.Infof("Created Destination Namespace: \"%s\"", ns.Name)

	return ns, nil
}

func (mc *migrationCreate) ensureDestPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	if mc.PVC == nil {
		PVC, err := mc.createDestinationPVC(ctx)
		if err != nil {
			return nil, err
		}
		mc.PVC = PVC
	} else {
		klog.Infof("Destination PVC: \"%s\" is found in Namespace: \"%s\" and is used to create replication destination",
			mc.PVC.Name, mc.PVC.Namespace)
	}

	return mc.PVC, nil
}

func (mc *migrationCreate) createDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	destPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.DestinationPVC,
			Namespace: mc.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      mc.AccessModes,
			StorageClassName: mc.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *mc.Capacity,
				},
			},
		},
	}

	if err := mc.client.Create(ctx, destPVC); err != nil {
		return nil, err
	}

	klog.Infof("Created Destination PVC: \"%s\" in NameSpace: \"%s\"",
		destPVC.Name, destPVC.Namespace)

	return destPVC, nil
}

func (mc *migrationCreate) getDestinationPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	destPVC := &corev1.PersistentVolumeClaim{}
	pvcInfo := types.NamespacedName{
		Namespace: mc.Namespace,
		Name:      mc.DestinationPVC,
	}
	err := mc.client.Get(ctx, pvcInfo, destPVC)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			klog.Infof("pvc: \"%s\" not found, creating the same", mc.DestinationPVC)
			return nil, nil
		}
		return nil, err
	}
	return destPVC, nil
}
