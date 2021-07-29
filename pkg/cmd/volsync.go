package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	volsyncLong = templates.LongDesc(`
	    VolSync is a command line tool for a volsync operator running in a Kubernetes cluster.
		VolSync asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. VolSync uses a ReplicationDestination and a ReplicationSource
		to replicate a volume. Data will be synced according to the configured sync schedule.
`)
	volsyncExplain = templates.LongDesc(`
        To start using VolSync, login to your cluster and install the VolSync operator.
		Installation instructions at https://volsync-replication.readthedocs.io/en/latest/installation/index.html

		For more on VolSync, see the documentation at https://volsync-replication.readthedocs.io/

		To see the full list of commands supported, run 'volsync --help'.`)

	volsyncConfig  = "config.yaml"
	VolSyncVersion = "0.0.0"
)

type ReplicationOptions struct {
	Source VolSyncSourceOptions
	Dest   VolSyncDestinationOptions

	genericclioptions.IOStreams
}

type Config struct {
	config string
}

type VolSyncSourceOptions struct {
	Config              Config
	KubeContext         string
	KubeClusterName     string
	Namespace           string
	Client              client.Client
	CopyMethod          volsyncv1alpha1.CopyMethodType
	Capacity            resource.Quantity
	StorageClass        *string
	AccessModes         []corev1.PersistentVolumeAccessMode
	VolumeSnapClassName *string
	SSHUser             *string
	ServiceType         corev1.ServiceType
	Port                *int32
	Provider            string
	Parameters          map[string]string
}

type VolSyncDestinationOptions struct {
	Config              Config
	KubeContext         string
	KubeClusterName     string
	Namespace           string
	Client              client.Client
	CopyMethod          volsyncv1alpha1.CopyMethodType
	Capacity            resource.Quantity
	StorageClass        *string
	AccessModes         []corev1.PersistentVolumeAccessMode
	VolumeSnapClassName *string
	SSHUser             *string
	ServiceType         corev1.ServiceType
	Port                *int32
	Provider            string
	Parameters          map[string]string
}

//nolint:lll
func (o *VolSyncSourceOptions) Bind(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.KubeContext, "source-kube-context", o.KubeContext, ""+
		"the name of the kubeconfig context to use for the destination cluster. Defaults to current-context.")
	flags.StringVar(&o.KubeClusterName, "source-kube-clustername", o.KubeClusterName, ""+
		"the name of the kubeconfig cluster to use for the destination cluster. Defaults to current cluster.")
	flags.StringVar(&o.Namespace, "source-namespace", o.Namespace, ""+
		"the transfer source namespace and/or location of a ReplicationSource. This namespace must exist. Defaults to current namespace.")
	flags.VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

//nolint:lll
func (o *VolSyncDestinationOptions) Bind(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.KubeContext, "dest-kube-context", o.KubeContext, ""+
		"the name of the kubeconfig context to use for the destination cluster. Defaults to current-context.")
	flags.StringVar(&o.KubeClusterName, "dest-kube-clustername", o.KubeClusterName, ""+
		"the name of the kubeconfig cluster to use for the destination cluster. Defaults to current-cluster.")
	flags.StringVar(&o.Namespace, "dest-namespace", o.Namespace, ""+
		"the transfer destination namespace and/or location of a ReplicationDestination. This namespace must exist. Defaults to current namespace.")
	flags.VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

//nolint:lll
func (o *Config) bindFlags(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.config, "config", o.config, ""+
		"the path to file holding flag values. If empty, looks for ./config.yaml then ~/.volsyncconfig/config.yaml. "+
		"Command line values override config file.")
	flags.VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

func (o *Config) complete(v *viper.Viper) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".volsyncconfig")
	v.SetConfigName(volsyncConfig)
	v.AddConfigPath(configPath)
	v.SetConfigType("yaml")
	configFile := filepath.Join(configPath, volsyncConfig)
	// check for config file flag, write to configFile
	if len(o.config) > 0 {
		if err = writeToConfig(o.config, configFile); err != nil {
			return err
		}
	} else {
		// check for config file in current directory, write to configFile
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		if _, err = os.Stat(filepath.Join(wd, volsyncConfig)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err = writeToConfig(filepath.Join(wd, volsyncConfig), configFile); err != nil {
				return err
			}
		}
	}
	if err = v.ReadInConfig(); err != nil {
		var nf *viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return err
		}
	}
	return nil
}

func (o *Config) Bind(cmd *cobra.Command, v *viper.Viper) error {
	o.bindFlags(cmd, v)
	if err := o.complete(v); err != nil {
		return err
	}
	return nil
}

func (o *ReplicationOptions) Bind(cmd *cobra.Command, v *viper.Viper) {
	o.Source.Bind(cmd, v)
	o.Dest.Bind(cmd, v)
}

// NewCmdVolSync implements the volsync command
func NewCmdVolSync(in io.Reader, out, errout io.Writer) *cobra.Command {
	// main command
	streams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: errout}
	volsynccmd := &cobra.Command{
		Use:     "volsync",
		Short:   "Asynchronously replicate persistent volumes.",
		Version: VolSyncVersion,
		Long:    fmt.Sprint(volsyncLong),
		Run: func(c *cobra.Command, args []string) {
			c.SetOutput(errout)
			kcmdutil.RequireNoArguments(c, args)
			fmt.Fprintf(errout, "%s\n\n%s\n", volsyncLong, volsyncExplain)
		},
	}
	volsynccmd.AddCommand(NewCmdVolSyncStartReplication(streams))
	volsynccmd.AddCommand(NewCmdVolSyncSetReplication(streams))
	volsynccmd.AddCommand(NewCmdVolSyncContinueReplication(streams))
	volsynccmd.AddCommand(NewCmdVolSyncRemoveReplication(streams))

	return volsynccmd
}

func (o *ReplicationOptions) Complete() error {
	err := o.Source.Complete()
	if err != nil {
		return err
	}
	err = o.Dest.Complete()
	if err != nil {
		return err
	}
	return nil
}

//nolint:dupl
func (o *VolSyncSourceOptions) Complete() error {
	sourceKubeConfigFlags := genericclioptions.NewConfigFlags(true)
	if len(o.KubeContext) > 0 {
		sourceKubeConfigFlags.Context = &o.KubeContext
	}
	if len(o.KubeClusterName) > 0 {
		sourceKubeConfigFlags.ClusterName = &o.KubeClusterName
	}
	sourcef := kcmdutil.NewFactory(sourceKubeConfigFlags)

	sourceClientConfig, err := sourcef.ToRESTConfig()
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(volsyncv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	sourceKClient, err := client.New(sourceClientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	o.Client = sourceKClient
	if len(o.Namespace) == 0 {
		o.Namespace, _, err = sourcef.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}
	}
	return nil
}

//nolint:dupl
func (o *VolSyncDestinationOptions) Complete() error {
	destKubeConfigFlags := genericclioptions.NewConfigFlags(true)
	if len(o.KubeContext) > 0 {
		destKubeConfigFlags.Context = &o.KubeContext
	}
	if len(o.KubeClusterName) > 0 {
		destKubeConfigFlags.ClusterName = &o.KubeClusterName
	}
	destf := kcmdutil.NewFactory(destKubeConfigFlags)

	// get client and namespace
	destClientConfig, err := destf.ToRESTConfig()
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(volsyncv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	destKClient, err := client.New(destClientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	o.Client = destKClient
	if len(o.Namespace) == 0 {
		o.Namespace, _, err = destf.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeToConfig(source, configFile string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	err = os.MkdirAll(filepath.Dir(configFile), 0755)
	if err != nil {
		return err
	}
	newFile, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, sourceFile)
	if err != nil {
		return err
	}
	klog.V(2).Infof("Wrote config %s to ~/.volsyncconfig/volsync-config.yaml", source)
	return nil
}
