package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type SSHKeysSecretOptions struct {
	Config        Config
	RepOpts       ReplicationOptions
	SSHKeysSecret string
}

//nolint:lll
func (o *SSHKeysSecretOptions) Bind(cmd *cobra.Command, v *viper.Viper) {
	flags := cmd.Flags()
	flags.StringVar(&o.SSHKeysSecret, "ssh-keys-secret", o.SSHKeysSecret, ""+
		"name of existing valid SSHKeys secret for authentication. If not set, the dest-src SSHKey secret-name will be used from destinationlocation.")

	flags.VisitAll(func(f *pflag.Flag) {
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
}

func (o *SSHKeysSecretOptions) SyncSSHSecret() error {
	ctx := context.Background()
	originalSecret := &corev1.Secret{}
	nsName := types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      o.SSHKeysSecret,
	}
	err := o.RepOpts.Dest.Client.Get(ctx, nsName, originalSecret)
	if err != nil {
		return err
	}
	newSecret := originalSecret.DeepCopy()
	newSecret.ObjectMeta = metav1.ObjectMeta{
		Name:            originalSecret.ObjectMeta.Name,
		Namespace:       o.RepOpts.Source.Namespace,
		OwnerReferences: nil,
	}

	err = o.RepOpts.Source.Client.Create(ctx, newSecret)
	if err != nil {
		return err
	}
	klog.Infof("Secret %s created in namespace %s", o.SSHKeysSecret, o.RepOpts.Source.Namespace)
	return nil
}
