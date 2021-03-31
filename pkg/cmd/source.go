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
	scribeNewSourceLong = templates.LongDesc(`
        Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. Scribe uses a ReplicationDestination and a ReplicationSource to
		replicate a volume. Data will be synced according to the configured sync schedule.
`)
	scribeNewSourceExample = templates.Examples(`
        # Create a ReplicationSource with 'scribe-config' file holding flag values in current directory.
        scribe new-source

        # Create a ReplicationSource for mysql-pvc using Snapshot copy method in the namespace 'source'.
        $ scribe new-source --source-namespace source --source-copy-method Snapshot --source-pvc mysql-pvc

        # Create a ReplicationSource for mysql-pvc using Snapshot copy method in the namespace 'source'
		# in clustername 'api-source-test-com:6443' with context 'user-scribe'.
        $ scribe new-source --source-namespace source \
		    --source-copy-method Snapshot --source-pvc mysql-pvc \
			--source-kube-context user-scribe --source-clustername api-source-test-com:6443

        # Create a ReplicationSource for mysql-pvc using Clone copy method in the current namespace.
        $ scribe new-source --source-copy-method Clone --source-pvc mysql-pvc
    `)
)

type SourceOptions struct {
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

func NewSourceOptions(streams genericclioptions.IOStreams) *SourceOptions {
	return &SourceOptions{
		IOStreams: streams,
	}
}

func NewCmdScribeNewSource(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewSourceOptions(streams)
	cmd := &cobra.Command{
		Use:     "new-source [OPTIONS]",
		Short:   i18n.T("Create a ReplicationSource for replicating a persistent volume."),
		Long:    fmt.Sprint(scribeNewSourceLong),
		Example: fmt.Sprint(scribeNewSourceExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.CreateReplicationSource())
		},
	}
	kcmdutil.CheckErr(o.ScribeOptions.Bind(cmd, v))
	kcmdutil.CheckErr(o.SSHKeysSecretOptions.Bind(cmd, v))
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

//nolint:lll
func (o *SourceOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "source-copy-method", o.CopyMethod, "the method of creating a point-in-time image of the source volume; one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Address, "address", o.Address, "the remote address to connect to for replication.")
	flags.StringVar(&o.Capacity, "source-capacity", o.Capacity, "provided to override the capacity of the point-in-Time image.")
	flags.StringVar(&o.StorageClassName, "source-storage-class-name", o.StorageClassName, "provided to override the StorageClass of the point-in-Time image.")
	flags.StringVar(&o.AccessMode, "source-access-mode", o.AccessMode, "provided to override the accessModes of the point-in-Time image. One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "source-volume-snapshot-class", o.VolumeSnapshotClassName, "name of VolumeSnapshotClass for the source volume, only if copyMethod is 'Snapshot'. If empty, default VSC will be used.")
	flags.StringVar(&o.PVC, "source-pvc", o.PVC, "name of an existing PersistentVolumeClaim (PVC) to replicate.")
	// TODO: Default to every 3min for source?
	flags.StringVar(&o.Schedule, "source-cron-spec", "*/3 * * * *", "cronspec to be used to schedule capturing the state of the source volume. If not set the source volume will be captured every 3 minutes.")
	// Defaults to "root" after creation
	flags.StringVar(&o.SSHUser, "source-ssh-user", o.SSHUser, "username for outgoing SSH connections (default 'root')")
	// Defaults to ClusterIP after creation
	flags.StringVar(&o.ServiceType, "source-service-type", o.ServiceType, "one of ClusterIP|LoadBalancer. Service type that will be created for incoming SSH connections. (default 'ClusterIP')")
	// TODO: Defaulted in CLI, should it be??
	flags.StringVar(&o.Name, "source-name", o.Name, "name of the ReplicationSource resource (default '<source-ns>-scribe-source')")
	// defaults to 22 after creation
	flags.Int32Var(&o.Port, "port", o.Port, "SSH port to connect to for replication. (default 22)")
	flags.StringVar(&o.Provider, "provider", o.Provider, "name of an external replication provider, if applicable; pass as 'domain.com/provider'")
	// TODO: I don't know how many params providers have? If a lot, can pass a file instead
	flags.StringVar(&o.ProviderParameters, "provider-parameters", o.ProviderParameters, "provider-specific key=value configuration parameters, for an external provider; pass 'key=value,key1=value1'")
	// defaults to "/" after creation
	flags.StringVar(&o.Path, "path", o.Path, "the remote path to rsync to (default '/')")
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

func (o *SourceOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
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

func (o *SourceOptions) Complete(cmd *cobra.Command) error {
	err := o.ScribeOptions.Complete()
	if err != nil {
		return err
	}
	o.Namespace = o.ScribeOptions.SourceNamespace
	if len(o.Name) == 0 {
		o.Name = o.Namespace + "-source"
	}
	klog.V(2).Infof("replication source %s will be created in %s namespace", o.Name, o.Namespace)
	return nil
}

//nolint:lll
// Validate validates ReplicationSource options.
func (o *SourceOptions) Validate() error {
	if len(o.CopyMethod) == 0 {
		return fmt.Errorf("must provide --copy-method; one of 'None|Clone|Snapshot'")
	}
	//TODO: FIX THIS to find default secret name, so this won't be required
	if len(o.SSHKeysSecretOptions.SSHKeysSecret) == 0 {
		return fmt.Errorf("must provide the name of the secret in ReplicationSource namespace that holds the SSHKeys for connecting to the ReplicationDestination namespace")
	}
	return nil
}

func (o *SourceOptions) commonOptions() (*CommonOptions, error) {
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

// CreateReplicationSource creates a ReplicationSource resource
func (o *SourceOptions) CreateReplicationSource() error {
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
	triggerSpec := &scribev1alpha1.ReplicationSourceTriggerSpec{
		Schedule: &o.Schedule,
	}
	if len(o.Schedule) == 0 {
		triggerSpec = nil
	}
	rsyncSpec := &scribev1alpha1.ReplicationSourceRsyncSpec{
		ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
			CopyMethod:              c.CopyMethod,
			Capacity:                c.Capacity,
			StorageClassName:        c.StorageClassName,
			AccessModes:             c.AccessModes,
			VolumeSnapshotClassName: c.VolumeSnapClassName,
		},
		SSHKeys:     sshKeysSecret,
		ServiceType: &c.ServiceType,
		Address:     c.Address,
		Port:        c.Port,
		Path:        c.Path,
		SSHUser:     c.SSHUser,
	}
	var externalSpec *scribev1alpha1.ReplicationSourceExternalSpec
	if len(o.Provider) > 0 && c.Parameters != nil {
		externalSpec = &scribev1alpha1.ReplicationSourceExternalSpec{
			Provider:   o.Provider,
			Parameters: c.Parameters,
		}
	}
	rs := &scribev1alpha1.ReplicationSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scribe.backube/v1alpha1",
			Kind:       "ReplicationSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
		},
		Spec: scribev1alpha1.ReplicationSourceSpec{
			SourcePVC: *c.PVC,
			Trigger:   triggerSpec,
			Rsync:     rsyncSpec,
			External:  externalSpec,
		},
	}
	if err := o.ScribeOptions.SourceClient.Create(context.TODO(), rs); err != nil {
		return err
	}
	klog.V(0).Infof("ReplicationSource %s created in namespace %s", o.Name, o.Namespace)
	return nil
}
