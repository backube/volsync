/*
Copyright 2020 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package rsync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/backube/volsync/controllers/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type rsyncSvcDescription struct {
	Context  context.Context
	Client   client.Client
	Service  *corev1.Service
	Owner    metav1.Object
	Type     *corev1.ServiceType
	Selector map[string]string
	Port     *int32
}

func (d *rsyncSvcDescription) Reconcile(l logr.Logger) error {
	logger := l.WithValues("service", client.ObjectKeyFromObject(d.Service))

	op, err := ctrlutil.CreateOrUpdate(d.Context, d.Client, d.Service, func() error {
		if err := ctrl.SetControllerReference(d.Owner, d.Service, d.Client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(d.Service)

		if d.Service.ObjectMeta.Annotations == nil {
			d.Service.ObjectMeta.Annotations = map[string]string{}
		}
		d.Service.ObjectMeta.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"] = "nlb"

		if d.Type != nil {
			d.Service.Spec.Type = *d.Type
		} else {
			d.Service.Spec.Type = corev1.ServiceTypeClusterIP
		}
		d.Service.Spec.Selector = d.Selector
		if len(d.Service.Spec.Ports) != 1 {
			d.Service.Spec.Ports = []corev1.ServicePort{{}}
		}
		d.Service.Spec.Ports[0].Name = "ssh"
		if d.Port != nil {
			d.Service.Spec.Ports[0].Port = *d.Port
		} else {
			d.Service.Spec.Ports[0].Port = 22
		}
		d.Service.Spec.Ports[0].Protocol = corev1.ProtocolTCP
		d.Service.Spec.Ports[0].TargetPort = intstr.FromInt(8022)
		if d.Service.Spec.Type == corev1.ServiceTypeClusterIP {
			d.Service.Spec.Ports[0].NodePort = 0
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Service reconcile failed")
		return err
	}

	logger.V(1).Info("Service reconciled", "operation", op)
	return nil
}

type rsyncSSHKeys struct {
	Context      context.Context
	Client       client.Client
	Owner        metav1.Object
	NameTemplate string
	MainSecret   *corev1.Secret
	SrcSecret    *corev1.Secret
	DestSecret   *corev1.Secret
}

func (k *rsyncSSHKeys) Reconcile(l logr.Logger) (bool, error) {
	k.MainSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.NameTemplate + "-main-" + k.Owner.GetName(),
			Namespace: k.Owner.GetNamespace(),
		},
	}
	k.SrcSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.NameTemplate + "-src-" + k.Owner.GetName(),
			Namespace: k.Owner.GetNamespace(),
		},
	}
	k.DestSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.NameTemplate + "-dest-" + k.Owner.GetName(),
			Namespace: k.Owner.GetNamespace(),
		},
	}
	return utils.ReconcileBatch(l,
		k.ensureMainSecret,
		k.ensureSrcSecret,
		k.ensureDestSecret,
	)
}

func (k *rsyncSSHKeys) ensureMainSecret(l logr.Logger) (bool, error) {
	// The secrets hold the ssh key pairs to ensure mutual authentication of the
	// connection. The main secret holds both keys and is used ensure the source
	// & destination secrets remain consistent with each other.
	//
	// Since the key generation creates unique keys each time it's run, we can't
	// do much to reconcile the main secret. All we can do is:
	// - Create it if it doesn't exist
	// - Ensure the expected fields are present within
	logger := l.WithValues("mainSecret", client.ObjectKeyFromObject(k.MainSecret))

	// See if it exists and has the proper fields
	err := k.Client.Get(k.Context, client.ObjectKeyFromObject(k.MainSecret), k.MainSecret)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Error(err, "failed to get secret")
		return false, err
	}
	if err == nil { // found it, make sure it has the right fields
		if utils.SecretHasFields(k.MainSecret, []string{"source", "source.pub", "destination", "destination.pub"}...) != nil {
			logger.V(1).Info("deleting invalid secret")
			if err = k.Client.Delete(k.Context, k.MainSecret); err != nil {
				logger.Error(err, "failed to delete secret")
			}
			return false, err
		}
		// Secret is valid, we're done
		logger.V(1).Info("secret is valid")
		return true, nil
	}

	// Need to create the secret
	if err = k.generateMainSecret(l); err != nil {
		l.Error(err, "unable to generate main secret")
		return false, err
	}
	if err = k.Client.Create(k.Context, k.MainSecret); err != nil {
		l.Error(err, "unable to create secret")
		return false, err
	}

	l.V(1).Info("created secret")
	return false, nil
}

func generateKeyPair(ctx context.Context, l logr.Logger) (private []byte, public []byte, err error) {
	keydir, err := os.MkdirTemp("", "sshkeys")
	if err != nil {
		l.Error(err, "unable to create temporary directory")
		return
	}
	defer os.RemoveAll(keydir)
	filename := filepath.Join(keydir, "key")
	if err = exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "rsa", "-b", "4096",
		"-f", filename, "-C", "", "-N", "").Run(); err != nil {
		return
	}
	if private, err = os.ReadFile(filename); err != nil {
		return
	}
	public, err = os.ReadFile(filename + ".pub")
	return
}

func (k *rsyncSSHKeys) generateMainSecret(l logr.Logger) error {
	k.MainSecret.Data = make(map[string][]byte, 4)
	if err := ctrl.SetControllerReference(k.Owner, k.MainSecret, k.Client.Scheme()); err != nil {
		l.Error(err, utils.ErrUnableToSetControllerRef)
		return err
	}
	utils.SetOwnedByVolSync(k.MainSecret)

	priv, pub, err := generateKeyPair(k.Context, l)
	if err != nil {
		l.Error(err, "unable to generate source ssh keys")
		return err
	}
	k.MainSecret.Data["source"] = priv
	k.MainSecret.Data["source.pub"] = pub

	priv, pub, err = generateKeyPair(k.Context, l)
	if err != nil {
		l.Error(err, "unable to generate destination ssh keys")
		return err
	}
	k.MainSecret.Data["destination"] = priv
	k.MainSecret.Data["destination.pub"] = pub

	l.V(1).Info("created secret")
	return nil
}

func (k *rsyncSSHKeys) ensureSecret(l logr.Logger, secret *corev1.Secret, keys []string) (bool, error) {
	logger := l.WithValues("secret", client.ObjectKeyFromObject(secret))

	op, err := ctrlutil.CreateOrUpdate(k.Context, k.Client, secret, func() error {
		if err := ctrl.SetControllerReference(k.Owner, secret, k.Client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(secret)
		if secret.Data == nil {
			secret.Data = make(map[string][]byte, 3)
		}
		for _, key := range keys {
			secret.Data[key] = k.MainSecret.Data[key]
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "reconcile failed")
	} else {
		logger.V(1).Info("reconciled", "operation", op)
	}
	return true, err
}

func (k *rsyncSSHKeys) ensureSrcSecret(l logr.Logger) (bool, error) {
	logger := l.WithValues("sourceSecret", client.ObjectKeyFromObject(k.SrcSecret))
	return k.ensureSecret(logger, k.SrcSecret, []string{"source", "source.pub", "destination.pub"})
}

func (k *rsyncSSHKeys) ensureDestSecret(l logr.Logger) (bool, error) {
	logger := l.WithValues("destSecret", client.ObjectKeyFromObject(k.DestSecret))
	return k.ensureSecret(logger, k.DestSecret, []string{"destination", "destination.pub", "source.pub"})
}
