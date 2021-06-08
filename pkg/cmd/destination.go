package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// This will be passed to scribe as 'nil' that will
// result in defaulting to the cluster's default storage class
var destPVCDefaultStorageClass = ""

type DestinationOptions struct {
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
	DestPVC                 string
	SSHUser                 string
	ServiceType             string
	Port                    int32
	Path                    string
	RcloneConfig            string
	Provider                string
	ProviderParameters      string
}

//nolint:lll
func (o *DestinationOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.CopyMethod, "dest-copy-method", o.CopyMethod, ""+
		"the method of creating a point-in-time image of the destination volume; one of 'None|Clone|Snapshot'")
	flags.StringVar(&o.Address, "dest-address", o.Address, "the remote address to connect to for replication.")
	// TODO: Defaulted with CLI, should it be??
	flags.StringVar(&o.Capacity, "dest-capacity", "2Gi", "Size of the destination volume to create. Must be provided if --dest-pvc is not provided.")
	flags.StringVar(&o.StorageClass, "dest-storage-class", o.StorageClass, ""+
		"name of the StorageClass of the destination volume. If not set, the default StorageClass will be used.")
	flags.StringVar(&o.AccessMode, "dest-access-mode", o.AccessMode, ""+
		"the access modes for the destination volume. Must be provided if --dest-pvc is not provided. "+
		"One of 'ReadWriteOnce|ReadOnlyMany|ReadWriteMany")
	flags.StringVar(&o.VolumeSnapshotClassName, "dest-volume-snapshot-class", o.VolumeSnapshotClassName, ""+
		"name of the VolumeSnapshotClass to be used for the destination volume, only if the copyMethod is 'Snapshot'. "+
		"If not set, the default VSC will be used.")
	flags.StringVar(&o.DestPVC, "dest-pvc", o.DestPVC, ""+
		"name of an existing empty PVC in the destination namespace to use as the transfer destination volume. If empty, one will be provisioned.")
	flags.StringVar(&o.Schedule, "dest-cron-spec", o.Schedule, ""+
		"cronspec to be used to schedule replication to occur at regular, time-based intervals. If not set replication will be continuous.")
	// Defaults to "root" after creation
	flags.StringVar(&o.SSHUser, "dest-ssh-user", o.SSHUser, "username for outgoing SSH connections (default 'root')")
	// Defaults to ClusterIP after creation
	flags.StringVar(&o.ServiceType, "dest-service-type", o.ServiceType, ""+
		"one of ClusterIP|LoadBalancer. Service type to be created for incoming SSH connections. (default 'ClusterIP')")
	// TODO: Defaulted in CLI, should it be??
	flags.StringVar(&o.Name, "dest-name", o.Name, "name of the ReplicationDestination resource. (default '<current-namespace>-scribe-destination')")
	flags.Int32Var(&o.Port, "dest-port", o.Port, "SSH port to connect to for replication. (default 22)")
	flags.StringVar(&o.Provider, "dest-provider", o.Provider, "name of an external replication provider, if applicable; pass as 'domain.com/provider'")
	// TODO: I don't know how many params providers have? If a lot, can pass a file instead
	flags.StringVar(&o.ProviderParameters, "dest-provider-parameters", o.ProviderParameters, ""+
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
