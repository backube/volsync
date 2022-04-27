package cmd

import (
	"context"
	"fmt"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForSync(ctx context.Context, srcClient client.Client,
	rsName types.NamespacedName, rs volsyncv1alpha1.ReplicationSource) error {
	err := wait.PollImmediate(5*time.Second, defaultVolumeSyncTimeout, func() (bool, error) {
		if err := srcClient.Get(ctx, rsName, &rs); err != nil {
			return false, err
		}
		if rs.Spec.Trigger == nil || rs.Spec.Trigger.Manual == "" {
			return false, fmt.Errorf("internal error: manual trigger not specified")
		}
		if rs.Status == nil {
			return false, nil
		}
		if rs.Status.LastManualSync != rs.Spec.Trigger.Manual {
			return false, nil
		}
		return true, nil
	})
	return err
}
