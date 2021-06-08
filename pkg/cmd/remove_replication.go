package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

var (
	scribeRemoveReplicationLong = templates.LongDesc(`
        Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. The remove-replication command will remove the scribe
		replication destination, replication source, and their resources. This command should be
		executed after the destination application is verified to be up-to-date and the destination PVC
		is bound to the destination application. The destination PVC and the source PVC are not modified.
		PVCs will never be deleted by Scribe.
`)
	scribeRemoveReplicationExample = templates.Examples(`
        # View all flags for remove-replication. 'scribe-config' can hold flag values.
		# Scribe config holds values for source PVC, source and destination context, and other options.
        $ scribe remove-replication --help

		# Remove a scribe replication and its resources. The destination PVC is not deleted or modified.
        $ scribe remove-replication

    `)
)

func NewCmdScribeRemoveReplication(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewFinalizeOptions(streams)
	cmd := &cobra.Command{
		Use:     "remove-replication [OPTIONS]",
		Short:   i18n.T("Remove a scribe replication and its resources."),
		Long:    fmt.Sprint(scribeRemoveReplicationLong),
		Example: fmt.Sprint(scribeRemoveReplicationExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.RemoveReplication())
		},
	}
	kcmdutil.CheckErr(o.Config.Bind(cmd, v))
	o.RepOpts.Bind(cmd, v)
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

//nolint:lll
// RemoveReplication does the following:
// 0) Checks ReplicationSource,Destination are connected (same rsync address)
// 1) Removes ReplicationSource
// 2) Removes synced sshSecret from source namespace
// 3) Removed ReplicationDestination
func (o *FinalizeOptions) RemoveReplication() error {
	ctx := context.Background()
	repSource := &scribev1alpha1.ReplicationSource{}
	sourceNSName := types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      o.sourceName,
	}
	if err := o.RepOpts.Source.Client.Get(ctx, sourceNSName, repSource); err != nil {
		return err
	}
	repDest := &scribev1alpha1.ReplicationDestination{}
	destNSName := types.NamespacedName{
		Namespace: o.RepOpts.Dest.Namespace,
		Name:      o.destName,
	}
	if err := o.RepOpts.Dest.Client.Get(ctx, destNSName, repDest); err != nil {
		return err
	}
	if *repSource.Spec.Rsync.Address != *repDest.Status.Rsync.Address {
		klog.Info("Refusing to remove replication, source and destination do not match")
		return fmt.Errorf(
			"Source RsyncAddress: %v does not match Destination RsyncAddress: %v",
			*repSource.Spec.Rsync.Address, *repDest.Status.Rsync.Address)
	}
	sshSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.RepOpts.Source.Namespace,
			Name:      *repSource.Spec.Rsync.SSHKeys,
		},
	}
	if err := o.RepOpts.Source.Client.Delete(ctx, sshSecret); err != nil {
		return fmt.Errorf("error deleting ssh-keys: %s namespace: %s: %w", sshSecret.Name, o.RepOpts.Source.Namespace, err)
	}
	klog.Infof("Deleted source SSH secret %s in namespace %s", sshSecret.Name, o.RepOpts.Source.Namespace)

	if err := o.RepOpts.Source.Client.Delete(ctx, repSource); err != nil {
		return fmt.Errorf("error deleting replication source: %s namespace: %s: %w", o.sourceName, o.RepOpts.Source.Namespace, err)
	}
	klog.Infof("Deleted replication source %s in namespace %s", o.sourceName, o.RepOpts.Source.Namespace)

	if err := o.RepOpts.Dest.Client.Delete(ctx, repDest); err != nil {
		return fmt.Errorf("error deleting replication destination: %s namespace: %s: %w", o.destName, o.RepOpts.Dest.Namespace, err)
	}
	klog.Infof("Deleted replication destination %s in namespace %s", o.destName, o.RepOpts.Dest.Namespace)

	klog.Infof("Scribe remove-replication complete.")
	return nil
}
