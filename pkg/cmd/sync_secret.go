package cmd

import (
	"context"
	"fmt"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scribeSyncSSHSecretLong = templates.LongDesc(`
        Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. Scribe uses a ReplicationDestination and a ReplicationSource
		to replicate a volume. Data will be synced according to the configured sync schedule.
`)
	scribeSyncSSHSecretExample = templates.Examples(`
	    # Copy the SSH secret from the ReplicationDestination namespace to the ReplicationSource namespace.
		# Secret will be copied from namespace 'dest' to namespace 'source'. Config file 'scribe-config'
		# in current directory with dest, source flag values.
		$ scribe sync-ssh-secret

		# Copy the SSH secret from the ReplicationDestination namespace to the ReplicationSource namespace.
		# Secret will be copied from namespace 'dest' to namespace 'source'.
		$ scribe sync-ssh-secret --dest-namespace=dest --source-namespace=source

		# Copy the SSH secret from the ReplicationDestination namespace in one cluster 
		# to the ReplicationSource namespace in another clutser.
		# Secret will be copied from clustername 'destcluster' context 'destuser' namespace 'dest'
		# to context 'sourceuser' clustername 'api-test-com:6443' namespace 'source'.
		$ scribe sync-ssh-secret \
		    --dest-namespace=dest \
		    --source-namespace=source \
			--dest-kube-context=destuser \
			--dest-clustername=destcluster \
			--source-kube-context=sourceuser \
			--source-clustername=api-test-com:6443
    `)
)

type SSHKeysSecretOptions struct {
	ScribeOptions ScribeOptions
	SSHKeysSecret string

	genericclioptions.IOStreams
}

func NewCmdScribeSyncSSHSecret(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewSSHKeysSecretOptions(streams)
	cmd := &cobra.Command{
		Use:     "sync-ssh-secret [OPTIONS]",
		Short:   i18n.T("Copy the SSH secret for rsync between namespaces and/or clusters."),
		Long:    fmt.Sprint(scribeSyncSSHSecretLong),
		Example: fmt.Sprint(scribeSyncSSHSecretExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.SyncSSHSecret())
		},
	}
	kcmdutil.CheckErr(o.ScribeOptions.Bind(cmd, v))
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

func (o *SSHKeysSecretOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
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
	o.bindFlags(cmd, v)
	return nil
}

//nolint:lll
func (o *SSHKeysSecretOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.SSHKeysSecret, "ssh-keys-secret", o.SSHKeysSecret, "name of existing valid SSHKeys secret for authentication. If not set, the dest-src SSHKey secret-name will be used from destinationlocation.")

	flags.VisitAll(func(f *pflag.Flag) {
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

func NewSSHKeysSecretOptions(streams genericclioptions.IOStreams) *SSHKeysSecretOptions {
	return &SSHKeysSecretOptions{
		IOStreams: streams,
	}
}

// Complete takes the cmd and infers options.
func (o *SSHKeysSecretOptions) Complete() error {
	ctx := context.Background()
	err := o.ScribeOptions.Complete()
	if err != nil {
		return err
	}
	repDests := &scribev1alpha1.ReplicationDestinationList{}
	opts := []client.ListOption{
		client.InNamespace(o.ScribeOptions.DestNamespace),
	}

	err = o.ScribeOptions.DestinationClient.List(ctx, repDests, opts...)
	if err != nil {
		return err
	}

	if len(o.SSHKeysSecret) == 0 {
		o.SSHKeysSecret = "scribe-rsync-dest-src-" + repDests.Items[0].Name
	}
	return nil
}

func (o *SSHKeysSecretOptions) SyncSSHSecret() error {
	ctx := context.Background()
	originalSecret := &corev1.Secret{}
	nsName := types.NamespacedName{
		Namespace: o.ScribeOptions.DestNamespace,
		Name:      o.SSHKeysSecret,
	}
	err := o.ScribeOptions.DestinationClient.Get(ctx, nsName, originalSecret)
	if err != nil {
		return err
	}
	newSecret := originalSecret.DeepCopy()
	newSecret.ObjectMeta = metav1.ObjectMeta{
		Name:            originalSecret.ObjectMeta.Name,
		Namespace:       o.ScribeOptions.SourceNamespace,
		OwnerReferences: nil,
	}

	err = o.ScribeOptions.SourceClient.Create(ctx, newSecret)
	if err != nil {
		return err
	}
	klog.Infof("secret %s created in namespace %s", o.SSHKeysSecret, o.ScribeOptions.SourceNamespace)
	return nil
}
