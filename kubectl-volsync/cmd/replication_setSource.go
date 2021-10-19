/*
Copyright © 2021 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

type replicationSetSource struct {
	rel *replicationRelationship
	// Parsed CLI options
	copyMethod volsyncv1alpha1.CopyMethodType
	pvcName    XClusterName
}

// replicationSetSourceCmd represents the replicationSetSource command
var replicationSetSourceCmd = &cobra.Command{
	Use:   "set-source",
	Short: i18n.T("Set the source of the replication"),
	Long: templates.LongDesc(i18n.T(`
	This command sets the source of the replication.
	`)),
	RunE: func(cmd *cobra.Command, args []string) error {
		rss, err := newReplicationSetSource(cmd)
		if err != nil {
			return err
		}
		rss.rel, err = loadReplicationRelationship(cmd)
		if err != nil {
			return err
		}
		return rss.Run(cmd.Context())
	},
}

func init() {
	replicationCmd.AddCommand(replicationSetSourceCmd)

	replicationSetSourceCmd.Flags().String("copymethod", "Clone", "method used to create a point-in-time copy")
	replicationSetSourceCmd.Flags().String("pvcname", "", "name of the PVC to replicate: [context/]namespace/name")
	cobra.CheckErr(replicationSetSourceCmd.MarkFlagRequired("pvcname"))
}

func newReplicationSetSource(cmd *cobra.Command) (*replicationSetSource, error) {
	rss := &replicationSetSource{}
	cm, err := cmd.Flags().GetString("copymethod")
	if err != nil {
		return nil, err
	}
	rss.copyMethod = volsyncv1alpha1.CopyMethodType(cm)
	if rss.copyMethod != volsyncv1alpha1.CopyMethodNone &&
		rss.copyMethod != volsyncv1alpha1.CopyMethodClone &&
		rss.copyMethod != volsyncv1alpha1.CopyMethodSnapshot {
		return nil, fmt.Errorf("unsupported copymethod: %v", rss.copyMethod)
	}

	pvcname, err := cmd.Flags().GetString("pvcname")
	if err != nil {
		return nil, err
	}
	xcr, err := ParseXClusterName(pvcname)
	if err != nil {
		return nil, err
	}
	rss.pvcName = *xcr

	return rss, nil
}

func (rss *replicationSetSource) Run(ctx context.Context) error {
	// Ensure PVC exists before we do anything
	pvcClient, err := newClient(rss.pvcName.Cluster)
	if err != nil {
		return err
	}

	pvc := corev1.PersistentVolumeClaim{}
	if err = pvcClient.Get(ctx, rss.pvcName.NamespacedName(), &pvc); err != nil {
		if kerrors.IsNotFound(err) {
			return fmt.Errorf("PVC %v not found in cluster context %s", rss.pvcName.NamespacedName(), rss.pvcName.Cluster)
		}
		return err
	}

	// Delete any previously defined source
	if rss.rel.data.Source != nil {
		srcClient, err := newClient(rss.rel.data.Source.Cluster)
		if err != nil {
			fmt.Printf("unable to create client for cluster context %s: %v", rss.rel.data.Source.Cluster, err)
		}
		if err = rss.deleteSource(ctx, srcClient); err != nil {
			if !kerrors.IsNotFound(err) {
				// We're unable to clean up the old, but maybe that's ok.
				fmt.Printf("unable to remove old ReplicationSource: %v", err)
			}
		}
	}

	rss.rel.data.Source = &replicationRelationshipSource{
		Cluster:   rss.pvcName.Cluster,
		Namespace: rss.pvcName.Namespace,
		RSName:    "", // Won't know the name until it's created
		PVCName:   rss.pvcName.Name,
		Source: volsyncv1alpha1.ReplicationSourceRsyncSpec{
			ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
				CopyMethod: rss.copyMethod,
			},
		},
	}

	if err = rss.rel.Save(); err != nil {
		return fmt.Errorf("unable to save relationship configuration: %w", err)
	}

	return nil
}

func (rss *replicationSetSource) deleteSource(ctx context.Context, c client.Client) error {
	if rss.rel.data.Source == nil || len(rss.rel.data.Source.RSName) == 0 {
		return nil
	}

	rs := volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rss.rel.data.Source.RSName,
			Namespace: rss.rel.data.Source.Namespace,
		},
	}

	err := c.Delete(ctx, &rs, client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err == nil || kerrors.IsNotFound(err) {
		rss.rel.data.Source.RSName = ""
	}
	return err
}
