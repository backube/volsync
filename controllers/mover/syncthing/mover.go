/*
Copyright 2021 The VolSync authors.

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

package syncthing

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	appsv1 "k8s.io/api/apps/v1"

	// "k8s.io/kubernetes/pkg/apis/apps"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
)

// constants used in the syncthing configuration
const (
	dataDirEnv             = "SYNCTHING_DATA_DIR"
	dataDirMountPath       = "/data"
	configDirEnv           = "SYNCTHING_CONFIG_DIR"
	configDirMountPath     = "/config"
	configPVCName          = "syncthing-config"
	syncthingJobName       = "syncthing"
	syncthingContainerName = "syncthing"
	syncthingAPIPort       = 8384
	syncthingDataPort      = 22000
	appLabelName           = "syncthing"
	apiKeySecretName       = "syncthing-apikey"
	apiServiceName         = "syncthing-api"
	dataServiceName        = "syncthing-data"
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client      client.Client
	logger      logr.Logger
	owner       metav1.Object
	isSource    bool
	paused      bool
	dataPVCName *string
	peerList    []v1alpha1.SyncthingPeer
	status      *v1alpha1.ReplicationSourceSyncthingStatus
	serviceType corev1.ServiceType
	syncthing   Syncthing
}

var _ mover.Mover = &Mover{}

// All object types that are temporary/per-iteration should be listed here. The
// individual objects to be cleaned up must also be marked.
var cleanupTypes = []client.Object{}

func (m *Mover) Name() string { return "syncthing" }

// We need the following resources available to us in the cluster:
// - PVC for syncthing-config
// - PVC that needs to be synced
// - Secret for the syncthing-apikey
// - Job/Pod running the syncthing mover image
// - Service exposing the syncthing REST API for us to make requests to
func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	var err error
	// ensure the data pvc exists
	if _, err = m.ensureDataPVC(ctx); err != nil {
		return mover.InProgress(), err
	}

	// create PVC for config data
	if _, err = m.ensureConfigPVC(ctx); err != nil {
		return mover.InProgress(), err
	}

	// ensure the secret exists
	if _, err = m.ensureSecretAPIKey(ctx); err != nil {
		return mover.InProgress(), err
	}

	if _, err = m.ensureDeployment(ctx); err != nil {
		return mover.InProgress(), err
	}
	// create the service for the syncthing REST API
	if _, err = m.ensureAPIService(ctx); err != nil {
		return mover.InProgress(), err
	}

	// ensure the external service exists
	if _, err = m.ensureDataService(ctx); err != nil {
		return mover.InProgress(), err
	}

	if _, err = m.ensureIsConfigured(ctx); err != nil {
		return mover.InProgress(), err
	}

	if err = m.ensureStatusIsUpdated(); err != nil {
		return mover.InProgress(), err
	}

	var retryAfter = 20 * time.Second
	return mover.RetryAfter(retryAfter), nil
}

func (m *Mover) ensureConfigPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	configPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(configPVC), configPVC); err == nil {
		// pvc already exists
		m.logger.Info("PVC already exists:  " + configPVC.Name)
		return configPVC, nil
	}

	// otherwise, create the PVC
	configPVC = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configPVCName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
	// set owner ref
	if err := ctrl.SetControllerReference(m.owner, configPVC, m.client.Scheme()); err != nil {
		m.logger.V(3).Error(err, "could not set owner ref")
		return nil, err
	}

	if err := m.client.Create(ctx, configPVC); err != nil {
		return nil, err
	}
	m.logger.Info("Created PVC", configPVC.Name, configPVC)
	return configPVC, nil
}

func (m *Mover) ensureDataPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	// check if the data PVC exists, error if it doesn't
	fmt.Printf("Checking for PVC %s\n", *m.dataPVCName)
	dataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.dataPVCName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(dataPVC), dataPVC); err != nil {
		// pvc doesn't exist
		return nil, err
	}
	return dataPVC, nil
}

func (m *Mover) ensureSecretAPIKey(ctx context.Context) (*corev1.Secret, error) {
	// check if the secret exists, error if it doesn't
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiKeySecretName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
	}
	err := m.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)

	if err != nil {
		// need to create the secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apiKeySecretName,
				Namespace: m.owner.GetNamespace(),
				Labels: map[string]string{
					"app": appLabelName,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				// base64 encode an empty string
				"apikey":   []byte("password123"),
				"username": []byte("bob"),
				"password": []byte("bob"),
			},
		}
		if err = ctrl.SetControllerReference(m.owner, secret, m.client.Scheme()); err != nil {
			m.logger.Error(err, "Error setting controller reference")
			return nil, err
		}
		if err := m.client.Create(ctx, secret); err != nil {
			m.logger.Error(err, "Error creating secret")
			return nil, err
		}
		m.logger.Info("Created secret", secret.Name, secret)
	}
	return secret, nil
}

//nolint:funlen
func (m *Mover) ensureDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	// same thing as ensureJob, except this creates a deployment instead of a job
	var configVolumeName, dataVolumeName string = "syncthing-config", "syncthing-data"
	var numReplicas int32 = 1

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syncthingJobName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appLabelName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": appLabelName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  syncthingContainerName,
							Image: syncthingContainerImage,
							Command: []string{
								"/entry.sh",
							},
							Args: []string{
								"run",
							},
							Env: []corev1.EnvVar{
								{
									Name:  configDirEnv,
									Value: configDirMountPath,
								},
								{
									Name:  dataDirEnv,
									Value: dataDirMountPath,
								},
								{
									Name: "STGUIAPIKEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: apiKeySecretName,
											},
											Key: "apikey",
										},
									},
								},
							},
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "api",
									ContainerPort: syncthingAPIPort,
								},
								{
									Name:          "data",
									ContainerPort: syncthingDataPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      configVolumeName,
									MountPath: configDirMountPath,
								},
								{
									Name:      dataVolumeName,
									MountPath: dataDirMountPath,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: configVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: configPVCName,
								},
							},
						},
						{
							Name: dataVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: *m.dataPVCName,
								},
							},
						},
					},
				},
			},
		},
	}
	// check if deployment already exists, if so, don't create it again
	err := m.client.Get(ctx, types.NamespacedName{Name: syncthingJobName, Namespace: m.owner.GetNamespace()}, deployment)
	if err != nil && errors.IsNotFound(err) {
		// set owner ref
		if err = ctrl.SetControllerReference(m.owner, deployment, m.client.Scheme()); err != nil {
			m.logger.V(3).Error(err, "failed to set owner reference")
			return nil, err
		}
		err = m.client.Create(ctx, deployment)
		if err != nil {
			return nil, err
		}
	}
	return deployment, nil
}

func (m *Mover) ensureAPIService(ctx context.Context) (*corev1.Service, error) {
	targetPort := "api"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiServiceName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabelName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       syncthingAPIPort,
					TargetPort: intstr.FromString(targetPort),
					Protocol:   "TCP",
				},
			},
		},
	}
	err := m.client.Get(ctx, client.ObjectKeyFromObject(service), service)
	if err == nil {
		// service already exists
		m.logger.Info("service already exists", "service", service.Name)
	} else {
		if err = ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
			m.logger.V(3).Error(err, "failed to set owner reference")
			return nil, err
		}
		if err := m.client.Create(ctx, service); err != nil {
			m.logger.Error(err, "error creating the service")
			return nil, err
		}
	}
	if m.syncthing.APIConfig.APIURL == "" {
		// get the service url
		m.syncthing.APIConfig.APIURL = fmt.Sprintf(
			"https://%s.%s:%d", apiServiceName, m.owner.GetNamespace(), syncthingAPIPort,
		)
	}
	return service, nil
}

func (m *Mover) ensureDataService(ctx context.Context) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataServiceName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": appLabelName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabelName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       syncthingDataPort,
					TargetPort: intstr.FromInt(syncthingDataPort),
					Protocol:   "TCP",
				},
			},
			Type: m.serviceType,
		},
	}
	err := m.client.Get(ctx, client.ObjectKeyFromObject(service), service)
	if err == nil {
		m.logger.Info("service already exists", "service", service.Name)
		if service.Status.LoadBalancer.Ingress != nil && len(service.Status.LoadBalancer.Ingress) > 0 {
			m.status.Address = "tcp://" + service.Status.LoadBalancer.Ingress[0].IP + ":" + strconv.Itoa(syncthingDataPort)
		}
		return service, nil
	}

	if err := ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
		m.logger.V(3).Error(err, "failed to set owner reference")
		return nil, err
	}
	if err := m.client.Create(ctx, service); err != nil {
		m.logger.Error(err, "error creating the service")
		return nil, err
	}
	if service.Status.LoadBalancer.Ingress != nil && len(service.Status.LoadBalancer.Ingress) > 0 {
		m.status.Address = "tcp://" + service.Status.LoadBalancer.Ingress[0].IP + ":" + strconv.Itoa(syncthingDataPort)
	}

	return service, nil
}

func (m *Mover) Cleanup(ctx context.Context) (mover.Result, error) {
	err := utils.CleanupObjects(ctx, m.client, m.logger, m.owner, cleanupTypes)
	if err != nil {
		return mover.InProgress(), err
	}
	return mover.Complete(), nil
}

// get the API key
func (m *Mover) getAPIKey(ctx context.Context) (string, error) {
	// get the syncthing-apikey secret
	if m.syncthing.APIConfig.APIKey == "" {
		secret := &corev1.Secret{}
		err := m.client.Get(ctx, client.ObjectKey{Name: apiKeySecretName, Namespace: m.owner.GetNamespace()}, secret)
		if err != nil {
			return "", err
		}
		m.syncthing.APIConfig.APIKey = string(secret.Data["apikey"])
		m.syncthing.APIConfig.GUIUser = string(secret.Data["username"])
		m.syncthing.APIConfig.GUIPassword = string(secret.Data["password"])
	}
	return m.syncthing.APIConfig.APIKey, nil
}

func (m *Mover) ensureIsConfigured(ctx context.Context) (mover.Result, error) {
	var err error
	// get the api key
	if _, err = m.getAPIKey(ctx); err != nil {
		return mover.InProgress(), err
	}
	// reconciles the Syncthing object
	err = m.syncthing.FetchLatestInfo()
	if err != nil {
		return mover.InProgress(), err
	}
	m.logger.V(4).Info("Syncthing config", "config", m.syncthing.Config)

	hasChanged := false

	// check if the syncthing is configured
	if m.syncthing.NeedsReconfigure(m.peerList) {
		m.logger.V(3).Info("Syncthing needs reconfiguration")

		m.syncthing.UpdateDevices(m.peerList)
		m.logger.V(4).Info("Syncthing config after configuration", "config", m.syncthing.Config)
		hasChanged = true
	}

	// set the user and password if not already set
	if m.syncthing.Config.GUI.User != m.syncthing.APIConfig.GUIUser &&
		m.syncthing.Config.GUI.Password != m.syncthing.APIConfig.GUIPassword {
		m.logger.V(3).Info("Syncthing needs user and password")
		m.syncthing.Config.GUI.User = m.syncthing.APIConfig.GUIUser
		m.syncthing.Config.GUI.Password = m.syncthing.APIConfig.GUIPassword
		hasChanged = true
	}

	if hasChanged {
		// update the config
		err := m.syncthing.UpdateSyncthingConfig()
		if err != nil {
			m.logger.Error(err, "error updating syncthing config")
			return mover.InProgress(), err
		}
	}
	return mover.Complete(), nil
}

func (m *Mover) ensureStatusIsUpdated() error {
	// get the current status
	err := m.syncthing.FetchLatestInfo()
	if err != nil {
		return err
	}

	m.status.DeviceID = m.syncthing.SystemStatus.MyID
	m.status.Peers = []v1alpha1.SyncthingPeerStatus{}

	// add the connected devices to the status
	for _, device := range m.peerList {
		if (device.ID == m.syncthing.SystemStatus.MyID) || (device.ID == "") {
			continue
		}

		// check connection status
		devStats, ok := m.syncthing.SystemConnections.Connections[device.ID]
		m.status.Peers = append(m.status.Peers, v1alpha1.SyncthingPeerStatus{
			ID:            device.ID,
			Address:       device.Address,
			Connected:     ok && devStats.Connected,
			InBytesTotal:  int32(devStats.InBytesTotal),
			OutBytesTotal: int32(devStats.OutBytesTotal),
		})
	}
	return nil
}
