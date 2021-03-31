package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

var (
	scribeNewDestinationLong = templates.LongDesc(`
	    Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. Scribe uses a ReplicationDestination and a ReplicationSource
		to replicate a volume. Data will be synced according to the configured sync schedule.
`)
	scribeNewDestinationExample = templates.Examples(`
        # Create a ReplicationDestination with 'scribe-config' file holding flag values in current directory.
        $ scribe new-destination

        # Create a ReplicationDestination in the namespace 'dest'.
        $ scribe new-destination --dest-namespace dest --dest-copy-method Snapshot --dest-access-mode ReadWriteOnce

        # Create a ReplicationDestination in current namespace with Snapshot copy method and pvc mysql-claim
        $ scribe new-destination  --dest-copy-method Snapshot --dest-pvc mysql-claim

		# Create a ReplicationDestination in the namespace 'dest' in cluster 'api-test-dest-com:6443' with context 'destuser'.
        $ scribe new-destination --dest-namespace dest \
		    --dest-copy-method Snapshot \
			--dest-access-mode ReadWriteOnce \
			--dest-kube-context destuser \
			--dest-kube-clustername api-test-dest-com:6443
    `)
)

type DestinationOptions struct {
	Name                    string
	Namespace               string
	ScribeOptions           ScribeOptions
	SSHKeysSecretOptions    SSHKeysSecretOptions
	Schedule                string
	CopyMethod              string //v1alpha1.CopyMethodType
	Capacity                string //*resource.Quantity
	StorageClassName        string
	AccessMode              string //[]corev1.PersistentVolumeAccessMode
	Address                 string
	VolumeSnapshotClassName string
	PVC                     string
	SSHUser                 string
	ServiceType             string //*corev1.ServiceType
	Port                    int32  //int32
	Path                    string
	RcloneConfig            string
	Provider                string
	ProviderParameters      string //map[string]string
	genericclioptions.IOStreams
}

func NewDestinationOptions(streams genericclioptions.IOStreams) *DestinationOptions {
	return &DestinationOptions{
		IOStreams: streams,
	}
}

func NewCmdScribeNewDestination(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewDestinationOptions(streams)
	cmd := &cobra.Command{
		Use:     "new-destination [OPTIONS]",
		Short:   i18n.T("Create a ReplicationDestination for replicating a persistent volume."),
		Long:    fmt.Sprint(scribeNewDestinationLong),
		Example: fmt.Sprint(scribeNewDestinationExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.CreateReplicationDestination())
		},
	}
	kcmdutil.CheckErr(o.ScribeOptions.Bind(cmd, v))
	kcmdutil.CheckErr(o.SSHKeysSecretOptions.Bind(cmd, v))
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

//nolint:lll
func (o *DestinationOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "dest-copy-method", o.CopyMethod, "the method of creating a point-in-time image of the destination volume; one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Address, "address", o.Address, "the remote address to connect to for replication.")
	// TODO: Defaulted with CLI, should it be??
	flags.StringVar(&o.Capacity, "dest-capacity", "2Gi", "Size of the destination volume to create. Must be provided if --dest-pvc is not provided.")
	flags.StringVar(&o.StorageClassName, "dest-storage-class-name", o.StorageClassName, "name of the StorageClass of the destination volume. If not set, the default StorageClass will be used.")
	flags.StringVar(&o.AccessMode, "dest-access-mode", o.AccessMode, "the access modes for the destination volume. Must be provided if --dest-pvc is not provided; One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "dest-volume-snapshot-class", o.VolumeSnapshotClassName, "name of the VolumeSnapshotClass to be used for the destination volume, only if the copyMethod is 'Snapshot'. If not set, the default VSC will be used.")
	flags.StringVar(&o.PVC, "dest-pvc", o.PVC, "name of an existing PVC to use as the transfer destination volume instead of automatically provisioning one.")
	flags.StringVar(&o.Schedule, "dest-cron-spec", o.Schedule, "cronspec to be used to schedule replication to occur at regular, time-based intervals. If not set replication will be continuous.")
	// Defaults to "root" after creation
	flags.StringVar(&o.SSHUser, "dest-ssh-user", o.SSHUser, "username for outgoing SSH connections (default 'root')")
	// Defaults to ClusterIP after creation
	flags.StringVar(&o.ServiceType, "dest-service-type", o.ServiceType, "one of ClusterIP|LoadBalancer. Service type to be created for incoming SSH connections. (default 'ClusterIP')")
	// TODO: Defaulted in CLI, should it be??
	flags.StringVar(&o.Name, "dest-name", o.Name, "name of the ReplicationDestination resource. (default '<current-namespace>-scribe-destination')")
	flags.Int32Var(&o.Port, "port", o.Port, "SSH port to connect to for replication. (default 22)")
	flags.StringVar(&o.Provider, "provider", o.Provider, "name of an external replication provider, if applicable; pass as 'domain.com/provider'")
	// TODO: I don't know how many params providers have? If a lot, can pass a file instead
	flags.StringVar(&o.ProviderParameters, "provider-parameters", o.ProviderParameters, "provider-specific key=value configuration parameters, for an external provider; pass 'key=value,key1=value1'")
	// defaults to "/" after creation
	flags.StringVar(&o.Path, "path", o.Path, "the remote path to rsync to (default '/')")
	if err := cmd.MarkFlagRequired("dest-copy-method"); err != nil {
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

func (o *DestinationOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	// config file in current directory
	// TODO: where to look for config file
	v.SetConfigName(scribeConfig)
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}
	return o.bindFlags(cmd, v)
}

func (o *DestinationOptions) Complete(cmd *cobra.Command) error {
	err := o.ScribeOptions.Complete()
	if err != nil {
		return err
	}
	o.Namespace = o.ScribeOptions.DestNamespace
	if len(o.Name) == 0 {
		o.Name = o.Namespace + "-destination"
	}
	klog.V(2).Infof("replication destination %s will be created in %s namespace", o.Name, o.Namespace)
	return nil
}

// Validate validates ReplicationDestination options.
func (o *DestinationOptions) Validate() error {
	if len(o.CopyMethod) == 0 {
		return fmt.Errorf("must provide --dest-copy-method; one of 'None|Clone|Snapshot'")
	}
	if len(o.Capacity) == 0 && len(o.PVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	if len(o.AccessMode) == 0 && len(o.PVC) == 0 {
		return fmt.Errorf("must either provide --dest-capacity & --dest-access-mode OR --dest-pvc")
	}
	return nil
}

func (o *DestinationOptions) commonOptions() (*CommonOptions, error) {
	repOpts := &ReplicationOptions{
		CopyMethod:              o.CopyMethod,
		Capacity:                o.Capacity,
		StorageClassName:        o.StorageClassName,
		AccessMode:              o.AccessMode,
		Address:                 o.Address,
		VolumeSnapshotClassName: o.VolumeSnapshotClassName,
		PVC:                     o.PVC,
		SSHUser:                 o.SSHUser,
		ServiceType:             o.ServiceType,
		Port:                    o.Port,
		Path:                    o.Path,
		RcloneConfig:            o.RcloneConfig,
		Provider:                o.Provider,
		ProviderParameters:      o.ProviderParameters,
	}
	return repOpts.GetCommonOptions()
}

// CreateReplicationDestination creates a ReplicationDestination resource
func (o *DestinationOptions) CreateReplicationDestination() error {
	c, err := o.commonOptions()
	if err != nil {
		return err
	}
	var sshKeysSecret *string
	switch {
	case len(o.SSHKeysSecretOptions.SSHKeysSecret) > 0:
		sshKeysSecret = &o.SSHKeysSecretOptions.SSHKeysSecret
	default:
		sshKeysSecret = nil
	}
	triggerSpec := &scribev1alpha1.ReplicationDestinationTriggerSpec{
		Schedule: &o.Schedule,
	}
	if len(o.Schedule) == 0 {
		triggerSpec = nil
	}
	rsyncSpec := &scribev1alpha1.ReplicationDestinationRsyncSpec{
		ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{
			CopyMethod:              c.CopyMethod,
			Capacity:                c.Capacity,
			StorageClassName:        c.StorageClassName,
			AccessModes:             c.AccessModes,
			VolumeSnapshotClassName: c.VolumeSnapClassName,
			DestinationPVC:          c.PVC,
		},
		SSHKeys:     sshKeysSecret,
		SSHUser:     c.SSHUser,
		Address:     c.Address,
		ServiceType: &c.ServiceType,
		Port:        c.Port,
		Path:        c.Path,
	}
	var externalSpec *scribev1alpha1.ReplicationDestinationExternalSpec
	if len(o.Provider) > 0 && c.Parameters != nil {
		externalSpec = &scribev1alpha1.ReplicationDestinationExternalSpec{
			Provider:   o.Provider,
			Parameters: c.Parameters,
		}
	}
	rd := &scribev1alpha1.ReplicationDestination{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scribe.backube/v1alpha1",
			Kind:       "ReplicationDestination",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
		},
		Spec: scribev1alpha1.ReplicationDestinationSpec{
			Trigger:  triggerSpec,
			Rsync:    rsyncSpec,
			External: externalSpec,
		},
	}
	if err := o.ScribeOptions.DestinationClient.Create(context.TODO(), rd); err != nil {
		return err
	}
	klog.V(0).Infof("ReplicationDestination %s created in namespace %s", o.Name, o.Namespace)
	return nil
}
