package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	cron "github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
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

func deleteSecret(ctx context.Context, ns types.NamespacedName, cl client.Client) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns.Name,
			Namespace: ns.Namespace,
		},
	}

	err := cl.Delete(ctx, secret)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			klog.Infof("secret %s not found, ignore", ns.Name)
			return nil
		}
		return fmt.Errorf("failed to delete secret %s, %w", ns.Name, err)
	}

	return nil
}

func parseCronSpec(cs string) (*string, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(cs); err != nil {
		return nil, err
	}

	return &cs, nil
}

func parseResticConfig(filename string) (*resticConfig, error) {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		klog.Infof("config filename %s not found", filename)
		return nil, err
	}
	v := viper.New()
	v.SetConfigFile(filename)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("unable to read in config file, %w", err)
	}

	return &resticConfig{
		Viper:    *v,
		filename: filename,
	}, nil
}
