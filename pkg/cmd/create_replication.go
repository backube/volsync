package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

var (
	scribeStartReplicationLong = templates.LongDesc(`
        Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. Scribe uses a ReplicationDestination and a ReplicationSource to
		replicate a volume. Data will be synced according to the configured sync schedule.
		The start-replication command will create a ReplicationDestination, ReplicationSource,
		synced SSH keys secret from destination to source, and destination PVC that is a copy of
		the source PVC, with specified modifications, such as storage-class. 
`)
	scribeStartReplicationExample = templates.Examples(`
        # View all flags for start-replication. 'scribe-config' can hold flag values.
		# Scribe config holds values for source PVC, source and destination context, and other options.
        $ scribe start-replication --help

        # Start a Replication with 'scribe-config' file holding flag values in current directory.
		# Scribe config holds values for source PVC, source and destination context, and other options.
		# You may also pass any flags as command line options. Command line options will override those
		# in the config file.
        $ scribe start-replication

    `)
)

type SetupReplicationOptions struct {
	Name                    string
	Config                  Config
	RepOpts                 ReplicationOptions
	SSHKeysSecretOptions    SSHKeysSecretOptions
	DestOpts                DestinationOptions
	SourcePVC               string
	Schedule                string
	CopyMethod              string
	Capacity                string
	StorageClass            string
	AccessMode              string
	VolumeSnapshotClassName string
	SSHUser                 string
	ServiceType             string
	Port                    int32
	RcloneConfig            string
	Provider                string
	ProviderParameters      string
	genericclioptions.IOStreams
}

func NewSetupReplicationOptions(streams genericclioptions.IOStreams) *SetupReplicationOptions {
	return &SetupReplicationOptions{
		IOStreams: streams,
	}
}

func NewCmdScribeStartReplication(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewSetupReplicationOptions(streams)
	cmd := &cobra.Command{
		Use:     "start-replication [OPTIONS]",
		Short:   i18n.T("Start a scribe replication for  a persistent volume."),
		Long:    fmt.Sprint(scribeStartReplicationLong),
		Example: fmt.Sprint(scribeStartReplicationExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.StartReplication())
		},
	}
	kcmdutil.CheckErr(o.Config.Bind(cmd, v))
	kcmdutil.CheckErr(o.DestOpts.Bind(cmd, v))
	o.RepOpts.Bind(cmd, v)
	o.SSHKeysSecretOptions.Bind(cmd, v)
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

//nolint:lll
func (o *SetupReplicationOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "source-copy-method", o.CopyMethod, "the method of creating a point-in-time image of the source volume. "+
		"one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Capacity, "source-capacity", o.Capacity, "provided to override the capacity of the point-in-Time image.")
	flags.StringVar(&o.StorageClass, "source-storage-class-name", o.StorageClass, "provided to override the StorageClass of the point-in-Time image.")
	flags.StringVar(&o.AccessMode, "source-access-mode", o.AccessMode, "provided to override the accessModes of the point-in-Time image. "+
		"One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "source-volume-snapshot-class", o.VolumeSnapshotClassName, ""+
		"name of VolumeSnapshotClass for the source volume, only if copyMethod is 'Snapshot'. If empty, default VSC will be used.")
	flags.StringVar(&o.SourcePVC, "source-pvc", o.SourcePVC, "name of an existing PersistentVolumeClaim (PVC) to replicate.")
	// TODO: Default to every 3min for source?
	flags.StringVar(&o.Schedule, "source-cron-spec", "*/5 * * * *", "cronspec to be used to schedule capturing the state of the source volume.")
	// Defaults to "root" after creation
	flags.StringVar(&o.SSHUser, "source-ssh-user", o.SSHUser, "username for outgoing SSH connections (default 'root')")
	// Defaults to ClusterIP after creation
	flags.StringVar(&o.ServiceType, "source-service-type", o.ServiceType, ""+
		"one of ClusterIP|LoadBalancer. Service type that will be created for incoming SSH connections. (default 'ClusterIP')")
	// TODO: Defaulted in CLI, should it be??
	flags.StringVar(&o.Name, "source-name", o.Name, "name of the ReplicationSource resource (default '<source-ns>-source')")
	// defaults to 22 after creation
	flags.Int32Var(&o.Port, "source-port", o.Port, "SSH port to connect to for replication. (default 22)")
	flags.StringVar(&o.Provider, "source-provider", o.Provider, "name of an external replication provider, if applicable. "+
		"Provide as 'domain.com/provider'")
	// TODO: I don't know how many params providers have? If a lot, can pass a file instead
	flags.StringVar(&o.ProviderParameters, "source-provider-parameters", o.ProviderParameters, ""+
		"provider-specific key=value configuration parameters, for an external provider; pass 'key=value,key1=value1'")
	// TODO: Defaulted with CLI, should it be??
	if err := cmd.MarkFlagRequired("source-copy-method"); err != nil {
		return err
	}
	if err := cmd.MarkFlagRequired("source-pvc"); err != nil {
		return err
	}
	flags.VisitAll(func(f *pflag.Flag) {
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
	return nil
}

func (o *SetupReplicationOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	v.SetConfigName(scribeConfig)
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		var nf *viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return err
		}
	}
	return o.bindFlags(cmd, v)
}

func (o *SetupReplicationOptions) Complete() error {
	if err := o.RepOpts.Complete(); err != nil {
		return err
	}
	if len(o.Name) == 0 {
		o.Name = o.RepOpts.Source.Namespace + "-source"
	}
	if len(o.DestOpts.Name) == 0 {
		o.DestOpts.Name = fmt.Sprintf("%s-destination", o.RepOpts.Dest.Namespace)
	}
	if len(o.DestOpts.StorageClass) == 0 {
		o.DestOpts.StorageClass = destPVCDefaultStorageClass
	}
	if err := o.Validate(); err != nil {
		return err
	}
	return nil
}

//nolint:lll
func (o *SetupReplicationOptions) Validate() error {
	if len(o.CopyMethod) == 0 {
		return fmt.Errorf("must provide --source-copy-method; one of 'None|Clone|Snapshot'")
	}
	if len(o.DestOpts.CopyMethod) == 0 {
		return fmt.Errorf("must provide --dest-copy-method; one of 'None|Clone|Snapshot'")
	}
	if len(o.DestOpts.Capacity) == 0 && len(o.DestOpts.DestPVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	if len(o.DestOpts.AccessMode) == 0 && len(o.DestOpts.DestPVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	return nil
}

func (o *SetupReplicationOptions) sourceCommonOptions() error {
	sharedOpts := &sharedOptions{
		CopyMethod:              o.CopyMethod,
		Capacity:                o.Capacity,
		StorageClass:            o.StorageClass,
		AccessMode:              o.AccessMode,
		VolumeSnapshotClassName: o.VolumeSnapshotClassName,
		SSHUser:                 o.SSHUser,
		ServiceType:             o.ServiceType,
		Port:                    o.Port,
		RcloneConfig:            o.RcloneConfig,
		Provider:                o.Provider,
		ProviderParameters:      o.ProviderParameters,
	}
	return o.getCommonOptions(sharedOpts, scribeSource)
}

func (o *SetupReplicationOptions) destCommonOptions() error {
	sharedOpts := &sharedOptions{
		CopyMethod:              o.DestOpts.CopyMethod,
		Capacity:                o.DestOpts.Capacity,
		StorageClass:            o.DestOpts.StorageClass,
		AccessMode:              o.DestOpts.AccessMode,
		VolumeSnapshotClassName: o.DestOpts.VolumeSnapshotClassName,
		SSHUser:                 o.DestOpts.SSHUser,
		ServiceType:             o.DestOpts.ServiceType,
		Port:                    o.DestOpts.Port,
		RcloneConfig:            o.DestOpts.RcloneConfig,
		Provider:                o.DestOpts.Provider,
		ProviderParameters:      o.DestOpts.ProviderParameters,
	}
	return o.getCommonOptions(sharedOpts, scribeDest)
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
	repDest := &scribev1alpha1.ReplicationDestination{}
	nsName := types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      o.DestOpts.Name,
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
		klog.Infof("Found ReplicationDestination RSync Address: %s", *repDest.Status.Rsync.Address)
		address = repDest.Status.Rsync.Address
		return true, nil
	})
	if err != nil {
		return err
	}

	var sshKeysSecret *string
	switch {
	case len(o.SSHKeysSecretOptions.SSHKeysSecret) > 0:
		sshKeysSecret = &o.SSHKeysSecretOptions.SSHKeysSecret
	default:
		s := fmt.Sprintf("scribe-rsync-dest-src-%s", repDest.Name)
		sshKeysSecret = &s
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

	triggerSpec := &scribev1alpha1.ReplicationSourceTriggerSpec{
		Schedule: &o.Schedule,
	}
	if len(o.Schedule) == 0 {
		triggerSpec = nil
	}
	rsyncSpec := &scribev1alpha1.ReplicationSourceRsyncSpec{
		ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
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
	var externalSpec *scribev1alpha1.ReplicationSourceExternalSpec
	if len(o.RepOpts.Source.Provider) > 0 && o.RepOpts.Source.Parameters != nil {
		externalSpec = &scribev1alpha1.ReplicationSourceExternalSpec{
			Provider:   o.RepOpts.Source.Provider,
			Parameters: o.RepOpts.Source.Parameters,
		}
	}
	rs := &scribev1alpha1.ReplicationSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scribe.backube/v1alpha1",
			Kind:       "ReplicationSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.RepOpts.Source.Namespace,
		},
		Spec: scribev1alpha1.ReplicationSourceSpec{
			SourcePVC: o.SourcePVC,
			Trigger:   triggerSpec,
			Rsync:     rsyncSpec,
			External:  externalSpec,
		},
	}
	if err := o.RepOpts.Source.Client.Create(ctx, rs); err != nil {
		return err
	}
	klog.Infof("ReplicationSource %s created in namespace %s", o.Name, o.RepOpts.Source.Namespace)
	return nil
}

// NameDestinationPVC returns the name that will be given to the destination PVC
func (o *SetupReplicationOptions) NameDestinationPVC(ctx context.Context) (string, error) {
	// can't have 2 PVCs in same ns, same name
	destPVCName := o.DestOpts.DestPVC
	if len(destPVCName) == 0 {
		destPVCName = o.SourcePVC
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

	if len(o.DestOpts.DestPVC) == 0 || destPVCExists {
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
		Name:      o.SourcePVC,
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
	if o.RepOpts.Dest.CopyMethod == scribev1alpha1.CopyMethodNone {
		destPVCName, err = o.CreateDestinationPVCFromSource(ctx, nil)
		if err != nil {
			return err
		}
	}
	var sshKeysSecret *string
	switch {
	case len(o.SSHKeysSecretOptions.SSHKeysSecret) > 0:
		sshKeysSecret = &o.SSHKeysSecretOptions.SSHKeysSecret
	default:
		sshKeysSecret = nil
	}
	triggerSpec := &scribev1alpha1.ReplicationDestinationTriggerSpec{
		Schedule: &o.DestOpts.Schedule,
	}
	if len(o.DestOpts.Schedule) == 0 {
		triggerSpec = nil
	}
	address := &o.DestOpts.Address
	if len(o.DestOpts.Address) == 0 {
		address = nil
	}
	path := &o.DestOpts.Path
	if len(o.DestOpts.Path) == 0 {
		path = nil
	}

	rsyncSpec := &scribev1alpha1.ReplicationDestinationRsyncSpec{
		ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{
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

	if o.RepOpts.Dest.CopyMethod != scribev1alpha1.CopyMethodNone && len(o.DestOpts.DestPVC) == 0 {
		rsyncSpec.ReplicationDestinationVolumeOptions.DestinationPVC = nil
	}
	var externalSpec *scribev1alpha1.ReplicationDestinationExternalSpec
	if len(o.RepOpts.Dest.Provider) > 0 && o.RepOpts.Dest.Parameters != nil {
		externalSpec = &scribev1alpha1.ReplicationDestinationExternalSpec{
			Provider:   o.RepOpts.Dest.Provider,
			Parameters: o.RepOpts.Dest.Parameters,
		}
	}
	rd := &scribev1alpha1.ReplicationDestination{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scribe.backube/v1alpha1",
			Kind:       "ReplicationDestination",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.DestOpts.Name,
			Namespace: o.RepOpts.Dest.Namespace,
		},
		Spec: scribev1alpha1.ReplicationDestinationSpec{
			Trigger:  triggerSpec,
			Rsync:    rsyncSpec,
			External: externalSpec,
		},
	}
	klog.V(2).Infof("Creating ReplicationDestination %s in namespace %s", o.DestOpts.Name, o.RepOpts.Dest.Namespace)
	if err := o.RepOpts.Dest.Client.Create(ctx, rd); err != nil {
		return err
	}
	klog.V(0).Infof("ReplicationDestination %s created in namespace %s", o.DestOpts.Name, o.RepOpts.Dest.Namespace)
	return nil
}
