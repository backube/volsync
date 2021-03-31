package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scribeLong = templates.LongDesc(`
	    Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. Scribe uses a ReplicationDestination and a ReplicationSource
		to replicate a volume. Data will be synced according to the configured sync schedule.
`)
	scribeExplain = templates.LongDesc(`
        To start using Scribe, login to your cluster and install the Scribe operator.
		Installation instructions at https://scribe-replication.readthedocs.io/en/latest/installation/index.html

		For more on Scribe, see the documentation at https://scribe-replication.readthedocs.io/

		To see the full list of commands supported, run 'scribe --help'.`)

	scribeConfig  = "config.yaml"
	ScribeVersion = "0.0.0"
)

type ScribeOptions struct {
	config                string
	destKubeContext       string
	sourceKubeContext     string
	destKubeClusterName   string
	sourceKubeClusterName string
	DestNamespace         string
	SourceNamespace       string
	DestinationClient     client.Client
	SourceClient          client.Client

	genericclioptions.IOStreams
}

//nolint:lll
func (o *ScribeOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.config, "config", o.config, "the path to file holding flag values. If empty, looks for ./config.yaml then ~/.scribeconfig/config.yaml. Command line values override config file.")
	flags.StringVar(&o.destKubeContext, "dest-kube-context", o.destKubeContext, "the name of the kubeconfig context to use for the destination cluster. Defaults to current-context.")
	flags.StringVar(&o.sourceKubeContext, "source-kube-context", o.sourceKubeContext, "the name of the kubeconfig context to use for the destination cluster. Defaults to current-context.")
	flags.StringVar(&o.destKubeClusterName, "dest-kube-clustername", o.destKubeClusterName, "the name of the kubeconfig cluster to use for the destination cluster. Defaults to current-cluster.")
	flags.StringVar(&o.sourceKubeClusterName, "source-kube-clustername", o.sourceKubeClusterName, "the name of the kubeconfig cluster to use for the destination cluster. Defaults to current cluster.")
	flags.StringVar(&o.DestNamespace, "dest-namespace", o.DestNamespace, "the transfer destination namespace and/or location of a ReplicationDestination. This namespace must exist. If not set, use the current namespace.")
	flags.StringVar(&o.SourceNamespace, "source-namespace", o.SourceNamespace, "the transfer source namespace and/or location of a ReplicationSource. This namespace must exist. default is current namespace.")
	flags.VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

func (o *ScribeOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".scribeconfig")
	v.SetConfigName(scribeConfig)
	v.AddConfigPath(configPath)
	v.SetConfigType("yaml")
	configFile := filepath.Join(configPath, scribeConfig)
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
		if _, err = os.Stat(filepath.Join(wd, scribeConfig)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err = writeToConfig(filepath.Join(wd, scribeConfig), configFile); err != nil {
				return err
			}
		}
	}
	if err = v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}
	o.bindFlags(cmd, v)
	return nil
}

// NewCmdScribe implements the scribe command
func NewCmdScribe(in io.Reader, out, errout io.Writer) *cobra.Command {
	// main command
	streams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: errout}
	scribecmd := &cobra.Command{
		Use:     "scribe",
		Short:   "Asynchronously replicate persistent volumes.",
		Version: ScribeVersion,
		Long:    fmt.Sprint(scribeLong),
		Run: func(c *cobra.Command, args []string) {
			c.SetOutput(errout)
			kcmdutil.RequireNoArguments(c, args)
			fmt.Fprintf(errout, "%s\n\n%s\n", scribeLong, scribeExplain)
		},
	}
	scribecmd.AddCommand(NewCmdScribeNewDestination(streams))
	scribecmd.AddCommand(NewCmdScribeNewSource(streams))
	scribecmd.AddCommand(NewCmdScribeSyncSSHSecret(streams))

	return scribecmd
}

func (o *ScribeOptions) Complete() error {
	destKubeConfigFlags := genericclioptions.NewConfigFlags(true)
	if len(o.destKubeContext) > 0 {
		destKubeConfigFlags.Context = &o.destKubeContext
	}
	if len(o.destKubeClusterName) > 0 {
		destKubeConfigFlags.ClusterName = &o.destKubeClusterName
	}
	sourceKubeConfigFlags := genericclioptions.NewConfigFlags(true)
	if len(o.sourceKubeContext) > 0 {
		sourceKubeConfigFlags.Context = &o.sourceKubeContext
	}
	if len(o.sourceKubeClusterName) > 0 {
		sourceKubeConfigFlags.ClusterName = &o.sourceKubeClusterName
	}
	destf := kcmdutil.NewFactory(destKubeConfigFlags)
	sourcef := kcmdutil.NewFactory(sourceKubeConfigFlags)

	// get client and namespace
	destClientConfig, err := destf.ToRESTConfig()
	if err != nil {
		return err
	}
	sourceClientConfig, err := sourcef.ToRESTConfig()
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(scribev1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	destKClient, err := client.New(destClientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	o.DestinationClient = destKClient
	sourceKClient, err := client.New(sourceClientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	o.SourceClient = sourceKClient
	if len(o.DestNamespace) == 0 {
		o.DestNamespace, _, err = destf.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}
	}
	if len(o.SourceNamespace) == 0 {
		o.SourceNamespace, _, err = sourcef.ToRawKubeConfigLoader().Namespace()
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
	klog.V(2).Infof("Wrote config %s to ~/.scribeconfig/scribe-config.yaml", source)
	return nil
}
