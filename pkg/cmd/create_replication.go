package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	kerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var (
	volsyncStartReplicationLong = templates.LongDesc(`
        VolSync is a command line tool for a volsync operator running in a Kubernetes cluster.
		VolSync asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. VolSync uses a ReplicationDestination and a ReplicationSource to
		replicate a volume. Data will be synced according to the configured sync schedule.
		The start-replication command will create a ReplicationDestination, ReplicationSource,
		synced SSH keys secret from destination to source, and destination PVC that is a copy of
		the source PVC, with specified modifications, such as storage-class. 
`)
	volsyncStartReplicationExample = templates.Examples(`
        # View all flags for start-replication. 'volsync-config' can hold flag values.
		# VolSync config holds values for source PVC, source and destination context, and other options.
        $ volsync start-replication --help

        # Start a Replication with 'volsync-config' file holding flag values in current directory.
		# VolSync config holds values for source PVC, source and destination context, and other options.
		# You may also pass any flags as command line options. Command line options will override those
		# in the config file.
        $ volsync start-replication

    `)
)

type SetupReplicationOptions struct {
	RepOpts ReplicationOptions
	Dest    DestinationOptions
	Source  SourceOptions

	genericclioptions.IOStreams
}

func NewSetupReplicationOptions(streams genericclioptions.IOStreams) *SetupReplicationOptions {
	return &SetupReplicationOptions{
		IOStreams: streams,
	}
}

func NewCmdVolSyncStartReplication(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewSetupReplicationOptions(streams)
	cmd := &cobra.Command{
		Use:     "start-replication [OPTIONS]",
		Short:   i18n.T("Start a volsync replication for  a persistent volume."),
		Long:    fmt.Sprint(volsyncStartReplicationLong),
		Example: fmt.Sprint(volsyncStartReplicationExample),
		Version: VolSyncVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.StartReplication())
		},
	}
	kcmdutil.CheckErr(o.Source.Config.Bind(cmd, v))
	kcmdutil.CheckErr(o.Dest.Bind(cmd, v))
	o.RepOpts.Bind(cmd, v)
	o.Source.SSHKeysSecretOptions.Bind(cmd, v)
	kcmdutil.CheckErr(o.Source.Bind(cmd, v))

	return cmd
}

func (o *SetupReplicationOptions) Complete() error {
	if err := o.RepOpts.Complete(); err != nil {
		return err
	}
	if len(o.Source.Name) == 0 {
		o.Source.Name = o.RepOpts.Source.Namespace + "-source"
	}
	if len(o.Dest.Name) == 0 {
		o.Dest.Name = fmt.Sprintf("%s-destination", o.RepOpts.Dest.Namespace)
	}
	if len(o.Dest.StorageClass) == 0 {
		o.Dest.StorageClass = destPVCDefaultStorageClass
	}
	if err := o.Validate(); err != nil {
		return err
	}
	return nil
}

//nolint:lll
func (o *SetupReplicationOptions) Validate() error {
	if len(o.Source.CopyMethod) == 0 {
		return fmt.Errorf("must provide --source-copy-method; one of 'None|Clone|Snapshot'")
	}
	if len(o.Dest.CopyMethod) == 0 {
		return fmt.Errorf("must provide --dest-copy-method; one of 'None|Clone|Snapshot'")
	}
	if len(o.Dest.Capacity) == 0 && len(o.Dest.PVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	if len(o.Dest.AccessMode) == 0 && len(o.Dest.PVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	return nil
}

func (o *SetupReplicationOptions) sourceCommonOptions() error {
	sharedOpts := &sharedOptions{
		CopyMethod:              o.Source.CopyMethod,
		Capacity:                o.Source.Capacity,
		StorageClass:            o.Source.StorageClass,
		AccessMode:              o.Source.AccessMode,
		VolumeSnapshotClassName: o.Source.VolumeSnapshotClassName,
		SSHUser:                 o.Source.SSHUser,
		ServiceType:             o.Source.ServiceType,
		Port:                    o.Source.Port,
		RcloneConfig:            o.Source.RcloneConfig,
		Provider:                o.Source.Provider,
		ProviderParameters:      o.Source.ProviderParameters,
	}
	return o.getCommonOptions(sharedOpts, volsyncSource)
}

func (o *SetupReplicationOptions) destCommonOptions() error {
	sharedOpts := &sharedOptions{
		CopyMethod:              o.Dest.CopyMethod,
		Capacity:                o.Dest.Capacity,
		StorageClass:            o.Dest.StorageClass,
		AccessMode:              o.Dest.AccessMode,
		VolumeSnapshotClassName: o.Dest.VolumeSnapshotClassName,
		SSHUser:                 o.Dest.SSHUser,
		ServiceType:             o.Dest.ServiceType,
		Port:                    o.Dest.Port,
		RcloneConfig:            o.Dest.RcloneConfig,
		Provider:                o.Dest.Provider,
		ProviderParameters:      o.Dest.ProviderParameters,
	}
	return o.getCommonOptions(sharedOpts, volsyncDest)
}

//nolint:funlen
// StartReplication does the following:
// 1) Create ReplicationDestination
// 2) Create DestinationPVC (if not provided)
// 3) Create ReplicationSource
func (o *SetupReplicationOptions) StartReplication() error {
	ctx := context.Background()
	if err := o.sourceCommonOptions(); err != nil {
		return err
	}
	if err := o.destCommonOptions(); err != nil {
		return err
	}
	if err := o.CreateDestination(ctx); err != nil {
		return err
	}

	klog.Infof("Extracting ReplicationDestination RSync address")
	repDest := &volsyncv1alpha1.ReplicationDestination{}
	nsName := types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      o.Dest.Name,
	}
	var address *string
	err := wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		err := o.RepOpts.Dest.Client.Get(ctx, nsName, repDest)
		if err != nil {
			return false, err
		}
		if repDest.Status == nil {
			return false, nil
		}
		if repDest.Status.Rsync.Address == nil {
			klog.Infof("Waiting for ReplicationDestination %s RSync address to populate", repDest.Name)
			return false, nil
		}

		if repDest.Status.Rsync.SSHKeys == nil {
			klog.Infof("Waiting for ReplicationDestination %s RSync sshkeys to populate", repDest.Name)
			return false, nil
		}

		klog.Infof("Found ReplicationDestination RSync Address: %s", *repDest.Status.Rsync.Address)
		address = repDest.Status.Rsync.Address
		return true, nil
	})
	if err != nil {
		return err
	}

	var sshKeysSecret *string
	switch {
	case len(o.Source.SSHKeysSecretOptions.SSHKeysSecret) > 0:
		sshKeysSecret = &o.Source.SSHKeysSecretOptions.SSHKeysSecret
	default:
		sshKeysSecret = repDest.Status.Rsync.SSHKeys
	}
	sshSecret := &corev1.Secret{}
	nsName = types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      *sshKeysSecret,
	}
	err = o.RepOpts.Dest.Client.Get(ctx, nsName, sshSecret)
	if err != nil {
		return fmt.Errorf("error retrieving destination sshSecret %s: %w", *sshKeysSecret, err)
	}
	klog.Infof("Found destination SSH secret %s, namespace %s", *sshKeysSecret, o.RepOpts.Dest.Namespace)
	sshSecret = &corev1.Secret{}
	nsName = types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      *sshKeysSecret,
	}
	klog.Infof("Ensuring source SSH secret %s exists in namespace %s", *sshKeysSecret, o.RepOpts.Source.Namespace)
	err = o.RepOpts.Dest.Client.Get(ctx, nsName, sshSecret)
	if err != nil {
		if !kerrs.IsNotFound(err) {
			return err
		}
		opts := &SSHKeysSecretOptions{
			RepOpts:       o.RepOpts,
			SSHKeysSecret: *sshKeysSecret,
		}
		err = opts.SyncSSHSecret()
		if err != nil {
			return err
		}
	}

	triggerSpec := &volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Schedule: &o.Source.Schedule,
	}
	if len(o.Source.Schedule) == 0 {
		triggerSpec = nil
	}
	rsyncSpec := &volsyncv1alpha1.ReplicationSourceRsyncSpec{
		ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
			CopyMethod:              o.RepOpts.Source.CopyMethod,
			Capacity:                &o.RepOpts.Source.Capacity,
			StorageClassName:        o.RepOpts.Source.StorageClass,
			AccessModes:             o.RepOpts.Source.AccessModes,
			VolumeSnapshotClassName: o.RepOpts.Source.VolumeSnapClassName,
		},
		SSHKeys:     sshKeysSecret,
		ServiceType: &o.RepOpts.Source.ServiceType,
		Address:     address,
		Port:        o.RepOpts.Source.Port,
		Path:        repDest.Spec.Rsync.Path,
		SSHUser:     o.RepOpts.Source.SSHUser,
	}
	var externalSpec *volsyncv1alpha1.ReplicationSourceExternalSpec
	if len(o.RepOpts.Source.Provider) > 0 && o.RepOpts.Source.Parameters != nil {
		externalSpec = &volsyncv1alpha1.ReplicationSourceExternalSpec{
			Provider:   o.RepOpts.Source.Provider,
			Parameters: o.RepOpts.Source.Parameters,
		}
	}
	rs := &volsyncv1alpha1.ReplicationSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "volsync.backube/v1alpha1",
			Kind:       "ReplicationSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Source.Name,
			Namespace: o.RepOpts.Source.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: o.Source.PVC,
			Trigger:   triggerSpec,
			Rsync:     rsyncSpec,
			External:  externalSpec,
		},
	}
	if err := o.RepOpts.Source.Client.Create(ctx, rs); err != nil {
		return err
	}
	klog.Infof("ReplicationSource %s created in namespace %s", o.Source.Name, o.RepOpts.Source.Namespace)
	return nil
}

// NameDestinationPVC returns the name that will be given to the destination PVC
func (o *SetupReplicationOptions) NameDestinationPVC(ctx context.Context) (string, error) {
	// can't have 2 PVCs in same ns, same name
	destPVCName := o.Dest.PVC
	if len(destPVCName) == 0 {
		destPVCName = o.Source.PVC
	}

	// check for PVC of same name in dest location. If so, then tag the new PVC to avoid name conflict
	destPVCExists := false
	destPVC := &corev1.PersistentVolumeClaim{}
	nsName := types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      destPVCName,
	}
	if err := o.RepOpts.Dest.Client.Get(ctx, nsName, destPVC); err == nil {
		destPVCExists = true
	} else {
		if !kerrs.IsNotFound(err) {
			return "", err
		}
	}

	if len(o.Dest.PVC) == 0 || destPVCExists {
		t := time.Now().Format("2006-01-02T15:04:05")
		tag := strings.ReplaceAll(t, ":", "-")
		destPVCName = strings.ToLower(fmt.Sprintf("%s-%s", destPVCName, tag))
	}
	return destPVCName, nil
}

func (o *SetupReplicationOptions) GetSourcePVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	srcPVC := &corev1.PersistentVolumeClaim{}
	nsName := types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      o.Source.PVC,
	}
	if err := o.RepOpts.Source.Client.Get(ctx, nsName, srcPVC); err != nil {
		return nil, err
	}
	return srcPVC, nil
}

// CreateDestinationPVCFromSource creates PVC in destination namespace synced from source PVC
func (o *SetupReplicationOptions) CreateDestinationPVCFromSource(
	ctx context.Context, latestImage *corev1.TypedLocalObjectReference) (string, error) {
	destPVCName, err := o.NameDestinationPVC(ctx)
	if err != nil {
		return "", err
	}
	srcPVC, err := o.GetSourcePVC(ctx)
	if err != nil {
		return "", err
	}
	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destPVCName,
			Namespace: o.RepOpts.Dest.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: o.RepOpts.Dest.StorageClass,
			AccessModes:      srcPVC.Spec.AccessModes,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: o.RepOpts.Dest.Capacity,
				},
			},
			Selector: srcPVC.Spec.Selector,
		},
	}

	// If ReplicationDestination is copy method Snapshot, add the DataSource
	// from the ReplicationDestination LatestImage
	if latestImage != nil {
		newPVC.Spec.DataSource = latestImage
	}

	klog.V(2).Infof("Creating PVC %s in destination namespace %s", destPVCName, o.RepOpts.Dest.Namespace)
	if err := o.RepOpts.Dest.Client.Create(ctx, newPVC); err != nil {
		return "", err
	}
	klog.Infof("PVC %s created in destination namespace: %s", destPVCName, o.RepOpts.Dest.Namespace)
	return destPVCName, nil
}

//nolint:funlen
// CreateDestination creates a ReplicationDestination resource
// along with a destination PVC if copyMethod "None"
func (o *SetupReplicationOptions) CreateDestination(ctx context.Context) error {
	var destPVCName string
	var err error
	if o.RepOpts.Dest.CopyMethod == volsyncv1alpha1.CopyMethodNone {
		destPVCName, err = o.CreateDestinationPVCFromSource(ctx, nil)
		if err != nil {
			return err
		}
	}
	var sshKeysSecret *string
	switch {
	case len(o.Source.SSHKeysSecretOptions.SSHKeysSecret) > 0:
		sshKeysSecret = &o.Source.SSHKeysSecretOptions.SSHKeysSecret
	default:
		sshKeysSecret = nil
	}
	triggerSpec := &volsyncv1alpha1.ReplicationDestinationTriggerSpec{
		Schedule: &o.Dest.Schedule,
	}
	if len(o.Dest.Schedule) == 0 {
		triggerSpec = nil
	}
	address := &o.Dest.Address
	if len(o.Dest.Address) == 0 {
		address = nil
	}
	path := &o.Dest.Path
	if len(o.Dest.Path) == 0 {
		path = nil
	}

	rsyncSpec := &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
		ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
			CopyMethod:              o.RepOpts.Dest.CopyMethod,
			Capacity:                &o.RepOpts.Dest.Capacity,
			StorageClassName:        o.RepOpts.Dest.StorageClass,
			AccessModes:             o.RepOpts.Dest.AccessModes,
			VolumeSnapshotClassName: o.RepOpts.Dest.VolumeSnapClassName,
			DestinationPVC:          &destPVCName,
		},
		SSHKeys:     sshKeysSecret,
		SSHUser:     o.RepOpts.Dest.SSHUser,
		Address:     address,
		ServiceType: &o.RepOpts.Dest.ServiceType,
		Port:        o.RepOpts.Dest.Port,
		Path:        path,
	}

	if o.RepOpts.Dest.CopyMethod != volsyncv1alpha1.CopyMethodNone && len(o.Dest.PVC) == 0 {
		rsyncSpec.ReplicationDestinationVolumeOptions.DestinationPVC = nil
	}
	var externalSpec *volsyncv1alpha1.ReplicationDestinationExternalSpec
	if len(o.RepOpts.Dest.Provider) > 0 && o.RepOpts.Dest.Parameters != nil {
		externalSpec = &volsyncv1alpha1.ReplicationDestinationExternalSpec{
			Provider:   o.RepOpts.Dest.Provider,
			Parameters: o.RepOpts.Dest.Parameters,
		}
	}
	rd := &volsyncv1alpha1.ReplicationDestination{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "volsync.backube/v1alpha1",
			Kind:       "ReplicationDestination",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Dest.Name,
			Namespace: o.RepOpts.Dest.Namespace,
		},
		Spec: volsyncv1alpha1.ReplicationDestinationSpec{
			Trigger:  triggerSpec,
			Rsync:    rsyncSpec,
			External: externalSpec,
		},
	}
	klog.V(2).Infof("Creating ReplicationDestination %s in namespace %s", o.Dest.Name, o.RepOpts.Dest.Namespace)
	if err := o.RepOpts.Dest.Client.Create(ctx, rd); err != nil {
		return err
	}
	klog.V(0).Infof("ReplicationDestination %s created in namespace %s", o.Dest.Name, o.RepOpts.Dest.Namespace)
	return nil
}
