package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// This will be passed to volsync as 'nil' that will
// result in defaulting to the cluster's default storage class
var destPVCDefaultStorageClass = ""

type ResourceOptions struct {
	Name                    string
	Config                  Config
	RepOpts                 ReplicationOptions
	SSHKeysSecretOptions    SSHKeysSecretOptions
	Schedule                string
	CopyMethod              string
	Capacity                string
	StorageClass            string
	AccessMode              string
	Address                 string
	VolumeSnapshotClassName string
	PVC                     string
	SSHUser                 string
	ServiceType             string
	Port                    int32
	Path                    string
	RcloneConfig            string
	Provider                string
	ProviderParameters      string
}

type SourceOptions struct {
	ResourceOptions
}

type DestinationOptions struct {
	ResourceOptions
}

//nolint:lll
func (o *DestinationOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "dest-copy-method", o.CopyMethod, ""+
		"the method of creating a point-in-time image of the destination volume; one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Address, "dest-address", o.Address, "the remote address to connect to for replication.")
	// TODO: Defaulted with CLI, should it be??
	flags.StringVar(&o.Capacity, "dest-capacity", "2Gi", "Size of the destination volume to create. Must be provided if --dest-pvc is not provided.")
	flags.StringVar(&o.StorageClass, "dest-storage-class-name", o.StorageClass, ""+
		"name of the StorageClass of the destination volume. If not set, the default StorageClass will be used.")
	flags.StringVar(&o.AccessMode, "dest-access-mode", o.AccessMode, ""+
		"the access modes for the destination volume. Must be provided if --dest-pvc is not provided. "+
		"One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "dest-volume-snapshot-class-name", o.VolumeSnapshotClassName, ""+
		"name of the VolumeSnapshotClass to be used for the destination volume, only if the copyMethod is 'Snapshot'. "+
		"If not set, the default VSC will be used.")
	flags.StringVar(&o.PVC, "dest-pvc", o.PVC, ""+
		"name of an existing empty PVC in the destination namespace to use as the transfer destination volume. If empty, one will be provisioned.")
	flags.StringVar(&o.Schedule, "dest-cron-spec", o.Schedule, ""+
		"cronspec to be used to schedule replication to occur at regular, time-based intervals. If not set replication will be continuous.")
	// Defaults to "root" after creation
	flags.StringVar(&o.SSHUser, "dest-ssh-user", o.SSHUser, "username for outgoing SSH connections (default 'root')")
	// Defaults to ClusterIP after creation
	flags.StringVar(&o.ServiceType, "dest-service-type", o.ServiceType, ""+
		"one of ClusterIP|LoadBalancer. Service type to be created for incoming SSH connections. (default 'ClusterIP')")
	// TODO: Defaulted in CLI, should it be??
	flags.StringVar(&o.Name, "dest-name", o.Name, "name of the ReplicationDestination resource. (default '<current-namespace>-volsync-destination')")
	flags.Int32Var(&o.Port, "dest-port", o.Port, "SSH port to connect to for replication. (default 22)")
	flags.StringVar(&o.Provider, "dest-provider", o.Provider, "name of an external replication provider, if applicable; pass as 'domain.com/provider'")
	// TODO: I don't know how many params providers have? If a lot, can pass a file instead
	flags.StringVar(&o.ProviderParameters, "dest-provider-params", o.ProviderParameters, ""+
		"provider-specific key=value configuration parameters, for an external provider; pass 'key=value,key1=value1'")
	// defaults to "/" after creation
	flags.StringVar(&o.Path, "dest-path", o.Path, "the remote path to rsync to (default '/')")
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

func (o *SourceOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	v.SetConfigName(volsyncConfig)
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		//nolint:errorlint
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}
	return o.bindFlags(cmd, v)
}

//nolint:lll
func (o *SourceOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "source-copy-method", o.CopyMethod, "the method of creating a point-in-time image of the source volume. "+
		"one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Capacity, "source-capacity", o.Capacity, "provided to override the capacity of the point-in-Time image.")
	flags.StringVar(&o.StorageClass, "source-storage-class-name", o.StorageClass, "provided to override the StorageClass of the point-in-Time image.")
	flags.StringVar(&o.AccessMode, "source-access-mode", o.AccessMode, "provided to override the accessModes of the point-in-Time image. "+
		"One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "source-volume-snapshot-class-name", o.VolumeSnapshotClassName, ""+
		"name of VolumeSnapshotClass for the source volume, only if copyMethod is 'Snapshot'. If empty, default VSC will be used.")
	flags.StringVar(&o.PVC, "source-pvc", o.PVC, "name of an existing PersistentVolumeClaim (PVC) to replicate.")
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
	flags.StringVar(&o.ProviderParameters, "source-provider-params", o.ProviderParameters, ""+
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
