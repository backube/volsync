//nolint:lll
package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

var (
	scribeSetReplicationLong = templates.LongDesc(`
        Scribe is a command line tool for a scribe operator running in a Kubernetes cluster.
		Scribe asynchronously replicates Kubernetes persistent volumes between clusters or namespaces
		using rsync, rclone, or restic. The set-replication command creates a PersistentVolumeClaim in
		the destination namespace with the latest image from the ReplicationDestination used as the
		data source for the PVC, in the case of destination CopyMethod=Snapshot.
		In the case of destination CopyMethod=None, the destination PVC is
		already created since the data is synced directly from source PVC to destination PVC.
		One more full data sync will be completed, then replications are paused. This leaves your
		destination application ready to bind to the destination PVC.
`)
	scribeSetReplicationExample = templates.Examples(`
        # View all flags for set-replication. 'scribe-config' can hold flag values.
		# Scribe config holds values for source PVC, source and destination context, and other options.
        $ scribe set-replication --help

        # Start a Replication with 'scribe-config' file holding flag values in current directory.
		# Scribe config holds values for source PVC, source and destination context, and other options.
		# You may also pass any flags as command line options. Command line options will override those
		# in the config file.
        $ scribe set-replication

    `)
)

type FinalizeOptions struct {
	Config           Config
	RepOpts          ReplicationOptions
	sourceName       string
	destName         string
	destPVC          string
	destStorageClass string
	destCapacity     string
	timeout          time.Duration
	genericclioptions.IOStreams
}

func NewFinalizeOptions(streams genericclioptions.IOStreams) *FinalizeOptions {
	return &FinalizeOptions{
		IOStreams: streams,
	}
}

func NewCmdScribeSetReplication(streams genericclioptions.IOStreams) *cobra.Command {
	v := viper.New()
	o := NewFinalizeOptions(streams)
	cmd := &cobra.Command{
		Use:     "set-replication [OPTIONS]",
		Short:   i18n.T("Set and pause a scribe replication and ensure a destination PVC with synced data."),
		Long:    fmt.Sprint(scribeSetReplicationLong),
		Example: fmt.Sprint(scribeSetReplicationExample),
		Version: ScribeVersion,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.SetReplication())
		},
	}
	kcmdutil.CheckErr(o.Config.Bind(cmd, v))
	o.RepOpts.Bind(cmd, v)
	kcmdutil.CheckErr(o.Bind(cmd, v))

	return cmd
}

func (o *FinalizeOptions) bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	flags := cmd.Flags()
	flags.StringVar(&o.sourceName, "source-replication-name", o.sourceName, "name of ReplicationSource (default '<source-ns>-source')")
	flags.StringVar(&o.destName, "dest-replication-name", o.destName, "name of ReplicationDestination (default '<dest-ns>-destination') ")
	flags.StringVar(&o.destPVC, "dest-pvc", o.destPVC, "name of not-yet-existing destination PVC. "+
		"Default is sourcePVC name, or if PVC with sourcePVC name exists in destination namespace, then 'sourcePVC-<date-tag>'")
	flags.StringVar(&o.destCapacity, "dest-capacity", o.destCapacity, "size of the destination volume to create. Default is source volume capacity.")
	flags.StringVar(&o.destStorageClass, "dest-storage-class", o.destStorageClass, ""+
		"name of the StorageClass of the destination volume. If not set, the default StorageClass will be used.")
	flags.DurationVar(&o.timeout, "timeout", time.Minute*5, "length of time to wait for final sync to complete. "+
		"Default is 5m. Pass values as time unit (e.g. 1,, 2m, 3h)")
	flags.VisitAll(func(f *pflag.Flag) {
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			kcmdutil.CheckErr(flags.Set(f.Name, fmt.Sprintf("%v", val)))
		}
	})
	return nil
}

func (o *FinalizeOptions) Bind(cmd *cobra.Command, v *viper.Viper) error {
	v.SetConfigName(scribeConfig)
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		var nf *viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return err
		}
	}
	return o.bindFlags(cmd, v)
}

func (o *FinalizeOptions) Complete() error {
	if err := o.RepOpts.Complete(); err != nil {
		return err
	}
	if len(o.destName) == 0 {
		o.destName = fmt.Sprintf("%s-destination", o.RepOpts.Dest.Namespace)
	}
	if len(o.sourceName) == 0 {
		o.sourceName = fmt.Sprintf("%s-source", o.RepOpts.Source.Namespace)
	}
	return nil
}

//nolint:funlen
// SetReplication does the following:
// 1) Performs manually triggered sync as the last sync
// 2) Create DestinationPVC if CopyMethod=Snapshot
// With the manual trigger in place, no further replications will execute.
func (o *FinalizeOptions) SetReplication() error {
	lastManualSync := time.Now().Format("2006-01-02t15-04-05")
	ctx := context.Background()
	klog.Infof("Fetching ReplicationSource %s in namespace %s", o.sourceName, o.RepOpts.Source.Namespace)
	repSource := &scribev1alpha1.ReplicationSource{}
	sourceNSName := types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      o.sourceName,
	}
	if err := o.RepOpts.Source.Client.Get(ctx, sourceNSName, repSource); err != nil {
		return err
	}
	klog.Infof("Triggering final data sync")
	repSource.Spec.Trigger = &scribev1alpha1.ReplicationSourceTriggerSpec{
		Schedule: repSource.Spec.Trigger.Schedule,
		Manual:   lastManualSync,
	}
	if err := o.RepOpts.Source.Client.Update(ctx, repSource); err != nil {
		return fmt.Errorf("unable to set manual trigger for last sync")
	}
	if err := wait.PollImmediate(5*time.Second, o.timeout, func() (bool, error) {
		if err := o.RepOpts.Source.Client.Get(ctx, sourceNSName, repSource); err != nil {
			return false, err
		}
		if repSource.Status.LastManualSync != lastManualSync {
			return false, nil
		}
		klog.Infof("Last data sync complete")
		return true, nil
	}); err != nil {
		return err
	}
	srcPVC := &corev1.PersistentVolumeClaim{}
	sourcePVCName := types.NamespacedName{
		Namespace: o.RepOpts.Source.Namespace,
		Name:      repSource.Spec.SourcePVC,
	}
	if err := o.RepOpts.Source.Client.Get(ctx, sourcePVCName, srcPVC); err != nil {
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
	var (
		latestImage *corev1.TypedLocalObjectReference
		destPVCName string
		err         error
	)
	if repDest.Spec.Rsync.CopyMethod == scribev1alpha1.CopyMethodNone {
		destPVCName = *repDest.Spec.Rsync.DestinationPVC
		if len(destPVCName) == 0 {
			return fmt.Errorf("destination PVC not listed in ReplicationDestination: %s", repDest.Name)
		}
	} else {
		latestImage = repDest.Status.LatestImage
		if latestImage == nil {
			return fmt.Errorf("ReplicationDestination does not have a latest snapshot image, exiting")
		}
		// if destPVC is empty, destination PVC name will be generated from source PVC
		destOpts := DestinationOptions{
			DestPVC: o.destPVC,
		}
		repOpts := &SetupReplicationOptions{
			RepOpts:      o.RepOpts,
			DestOpts:     destOpts,
			SourcePVC:    srcPVC.Name,
			StorageClass: o.destStorageClass,
		}
		if len(o.destCapacity) == 0 {
			repOpts.RepOpts.Dest.Capacity = srcPVC.Spec.Resources.Requests[corev1.ResourceStorage]
		} else {
			if err = repOpts.getCapacity(o.destCapacity, scribeDest); err != nil {
				return err
			}
		}
		destPVCName, err = repOpts.CreateDestinationPVCFromSource(ctx, latestImage)
		if err != nil {
			return err
		}
	}

	klog.Infof("Scribe set-replication complete.")
	klog.Infof("Scribe data sync complete. Destination CopyMethod %v.", repSource.Spec.Rsync.CopyMethod)
	klog.Infof("Replications paused until manual trigger is removed from source %s", repSource.Name)
	klog.Infof("It is now possible to edit the destination application to connect to the destination PVC: %s", destPVCName)
	klog.Info("Run 'continue-replication' to unpause and continue replications")
	return nil
}
