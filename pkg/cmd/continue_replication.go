package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var (
	volsyncContinueReplicationLong = templates.LongDesc(`
        VolSync is a command line tool for a volsync operator running in a Kubernetes cluster.
		VolSync asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		The 'continue' command will remove a manual trigger from a replication source and replication
		will resume according to the replication source schedule. Subsequent execution of 'set-replication'
		will result in a new destination PVC. PVCs will never be deleted by VolSync.
`)
	volsyncContinueReplicationExample = templates.Examples(`
        # View all flags for continue-replication. 'volsync-config' can hold flag values.
		# VolSync config holds values for source PVC, source and destination context, and other options.
        $ volsync continue-replication --help

		# Remove the manual trigger from a SourceDestination to resume replications.
		# This command should be run after ensuring the destination application is up-to-date.
        $ volsync continue

    `)
)

func NewCmdVolSyncContinueReplication(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewFinalizeOptions(streams)
	cmd := &cobra.Command{
		Use:     "continue-replication [OPTIONS]",
		Short:   i18n.T("remove a manual trigger from a volsync replication source to resume replications."),
		Long:    fmt.Sprint(volsyncContinueReplicationLong),
		Example: fmt.Sprint(volsyncContinueReplicationExample),
		Version: VolSyncVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Continue())
		},
	}
	kcmdutil.CheckErr(o.Config.Bind(cmd, v))
	o.RepOpts.Bind(cmd, v)
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

//nolint:lll
// Continue updates ReplicationSource to remove a manual trigger
// the replications then proceed according to the replication source schedule.
func (o *FinalizeOptions) Continue() error {
	ctx := context.Background()
	klog.Infof("Fetching ReplicationSource %s in namespace %s", o.sourceName, o.RepOpts.Source.Namespace)
	repSource := &volsyncv1alpha1.ReplicationSource{}
	sourceNSName := types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      o.sourceName,
	}
	if err := o.RepOpts.Source.Client.Get(ctx, sourceNSName, repSource); err != nil {
		return err
	}
	klog.Infof("Removing manual trigger from ReplicationSource: %s namespace: %s", o.RepOpts.Source.Namespace, o.sourceName)
	repSource.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
		Schedule: repSource.Spec.Trigger.Schedule,
	}
	if err := o.RepOpts.Source.Client.Update(ctx, repSource); err != nil {
		return fmt.Errorf("unable to remove manual trigger for last sync: %w", err)
	}
	klog.Infof("ReplicationSource schedule %v restored, manual trigger removed.", *repSource.Spec.Trigger.Schedule)
	return nil
}
