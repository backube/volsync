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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"
)

// constants used in the syncthing configuration
const (
	dataDirEnv         = "SYNCTHING_DATA_DIR"
	dataDirMountPath   = "/data"
	configDirEnv       = "SYNCTHING_CONFIG_DIR"
	configDirMountPath = "/config"
	configCapacity     = "1Gi"
	syncthingAPIPort   = 8384
	syncthingDataPort  = 22000
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client             client.Client
	logger             logr.Logger
	owner              client.Object
	eventRecorder      events.EventRecorder
	configStorageClass *string
	configAccessModes  []corev1.PersistentVolumeAccessMode
	containerImage     string
	paused             bool
	dataPVCName        *string
	peerList           []volsyncv1alpha1.SyncthingPeer
	status             *volsyncv1alpha1.ReplicationSourceSyncthingStatus
	serviceType        corev1.ServiceType
	syncthing          Syncthing
}

var _ mover.Mover = &Mover{}

// Name Returns the name of the mover.
func (m *Mover) Name() string { return "syncthing" }

// We need the following resources available to us in the cluster:
// - PVC for syncthing-config
// - PVC that needs to be synced
// - Secret for the syncthing-apikey
// - Syncthing mover deployment
// - Syncthing ClusterIP service exposing API
// - Service where the data service can be reached
//nolint:funlen
func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	var err error
	dataPVC, err := m.ensureDataPVC(ctx)
	if dataPVC == nil || err != nil {
		return mover.InProgress(), err
	}

	configPVC, err := m.ensureConfigPVC(ctx, dataPVC)
	if configPVC == nil || err != nil {
		return mover.InProgress(), err
	}

	secretAPIKey, err := m.ensureSecretAPIKey(ctx)
	if secretAPIKey == nil || err != nil {
		return mover.InProgress(), err
	}

	sa, err := m.ensureSA(ctx)
	if sa == nil || err != nil {
		return mover.InProgress(), err
	}

	deployment, err := m.ensureDeployment(ctx, dataPVC, configPVC, sa, secretAPIKey)
	if deployment == nil || err != nil {
		return mover.InProgress(), err
	}

	APIService, err := m.ensureAPIService(ctx, deployment)
	if APIService == nil || err != nil {
		return mover.InProgress(), err
	}

	dataService, err := m.ensureDataService(ctx)
	if dataService == nil || err != nil {
		return mover.InProgress(), err
	}

	if err = m.configureSyncthingAPIClient(secretAPIKey); err != nil {
		return mover.InProgress(), err
	}

	// configure syncthing before grabbing info & updating status
	if err = m.ensureIsConfigured(secretAPIKey); err != nil {
		return mover.InProgress(), err
	}

	if err = m.ensureStatusIsUpdated(dataService); err != nil {
		return mover.InProgress(), err
	}

	var retryAfter = 20 * time.Second
	return mover.RetryAfter(retryAfter), nil
}

// ensureConfigPVC Ensures that there is a PVC persisting Syncthing's config data.
func (m *Mover) ensureConfigPVC(
	ctx context.Context,
	dataPVC *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	capacity := resource.MustParse(configCapacity)
	options := volsyncv1alpha1.ReplicationSourceVolumeOptions{
		CopyMethod: volsyncv1alpha1.CopyMethodDirect,
		Capacity:   &capacity,
	}

	// ensure configStorageClassName
	if m.configStorageClass != nil {
		options.StorageClassName = m.configStorageClass
	} else {
		options.StorageClassName = dataPVC.Spec.StorageClassName
	}

	// ensure AccessModes
	if m.configAccessModes != nil {
		options.AccessModes = m.configAccessModes
	} else {
		options.AccessModes = dataPVC.Spec.AccessModes
	}

	configVh, err := volumehandler.NewVolumeHandler(
		volumehandler.WithClient(m.client),
		volumehandler.WithOwner(m.owner),
		volumehandler.WithRecorder(m.eventRecorder),
		volumehandler.FromSource(&options),
	)
	if err != nil {
		return nil, err
	}

	// Allocate the config volume
	configName := "volsync-" + m.owner.GetName() + "-config"
	m.logger.Info("allocating config volume", "PVC", configName)
	return configVh.EnsureNewPVC(ctx, m.logger, configName)
}

// ensureDataPVC Ensures that the PVC holding the data meant to be synced is available.
// A VolumeHandler will be created based on the provided source PVC.
func (m *Mover) ensureDataPVC(ctx context.Context) (*corev1.PersistentVolumeClaim, error) {
	// check if the data PVC exists, error if it doesn't
	dataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *m.dataPVCName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(dataPVC), dataPVC); err != nil {
		return nil, err
	}

	return dataPVC, nil
}

// ensureSecretAPIKey Ensures ensures that the PVC for API secrets either exists or it will create it.
//nolint:funlen
func (m *Mover) ensureSecretAPIKey(ctx context.Context) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	err := m.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)

	// need to create the secret
	if err != nil {
		// these will fail only when there is an issue with the OS's RNG
		randomAPIKey, err := GenerateRandomString(32)
		if err != nil {
			m.logger.Error(err, "could not generate random number")
			return nil, err
		}
		randomPassword, err := GenerateRandomString(32)
		if err != nil {
			m.logger.Error(err, "could not generate random number")
			return nil, err
		}

		// Generate TLS Certificates for communicating between VolSync and the Syncthing API
		apiServiceDNS := m.getAPIServiceDNS()
		certPEM, certPrivKeyPEM, err := GenerateTLSCertificatesForSyncthing(apiServiceDNS)
		if err != nil {
			m.logger.Error(err, "could not generate TLS certificate")
			return nil, err
		}

		// create a new secret with the generated values
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "volsync-" + m.owner.GetName(),
				Namespace: m.owner.GetNamespace(),
				Labels: map[string]string{
					"app": m.owner.GetName(),
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"apikey":       []byte(randomAPIKey),
				"username":     []byte("syncthing"),
				"password":     []byte(randomPassword),
				"httpsCertPEM": certPEM.Bytes(),
				"httpsKeyPEM":  certPrivKeyPEM.Bytes(),
			},
		}

		// ensure secret can be deleted once ReplicationSource is deleted
		if err = ctrl.SetControllerReference(m.owner, secret, m.client.Scheme()); err != nil {
			m.logger.Error(err, "could not set owner ref")
			return nil, err
		}

		if err := m.client.Create(ctx, secret); err != nil {
			return nil, err
		}
		m.logger.Info("created secret", secret.Name, secret)
	}
	return secret, nil
}

// ensureSA Ensures a VolSync ServiceAccount to be used by the operator.
func (m *Mover) ensureSA(ctx context.Context) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "volsync-src-" + m.owner.GetName(),
			Namespace: m.owner.GetNamespace(),
		},
	}
	saDesc := utils.NewSAHandler(ctx, m.client, m.owner, sa)
	cont, err := saDesc.Reconcile(m.logger)
	if cont {
		return sa, err
	}
	return nil, err
}

// ensureDeployment Will ensure that a Deployment for the Syncthing mover exists, or it will be created.
//nolint:funlen
func (m *Mover) ensureDeployment(ctx context.Context, dataPVC *corev1.PersistentVolumeClaim,
	configPVC *corev1.PersistentVolumeClaim, sa *corev1.ServiceAccount,
	apiSecret *corev1.Secret) (*appsv1.Deployment, error) {
	// same thing as ensureJob, except this creates a deployment instead of a job
	var configVolumeName, dataVolumeName string = "syncthing-config", "syncthing-data"
	var numReplicas int32 = 1

	deploymentName := "volsync-" + m.owner.GetName()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": m.owner.GetName(),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": m.owner.GetName(),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": m.owner.GetName(),
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa.Name,
					RestartPolicy:      corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "syncthing",
							Image: m.containerImage,
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
								// tell the mover image where to find the HTTPS certs
								{
									Name:  "SYNCTHING_CERT_DIR",
									Value: "/certs",
								},
								{
									Name: "STGUIAPIKEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: apiSecret.Name,
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
								{
									Name:      "https-certs",
									MountPath: "/certs",
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
									ClaimName: configPVC.Name,
								},
							},
						},
						{
							Name: dataVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dataPVC.Name,
								},
							},
						},
						// load the HTTPS certs as a volume
						{
							Name: "https-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: apiSecret.Name,
									Items: []corev1.KeyToPath{
										{
											Key:  "httpsKeyPEM",
											Path: "https-key.pem",
										},
										{
											Key:  "httpsCertPEM",
											Path: "https-cert.pem",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := m.client.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: m.owner.GetNamespace()}, deployment)
	if err != nil && errors.IsNotFound(err) {
		// ensure everything gets cleaned up after owner gets deleted
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

// ensureAPIService Ensures that a service exposing the Syncthing API is present, else it will be created.
func (m *Mover) ensureAPIService(ctx context.Context, deployment *appsv1.Deployment) (*corev1.Service, error) {
	// setup vars
	targetPort := "api"
	serviceName := m.getAPIServiceName()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: m.owner.GetNamespace(),
		},
	}

	// set API url if one is not already set
	m.syncthing.APIConfig.SetEmptyAPIURL(m.getAPIServiceAddress())

	// see if we already have a service
	err := m.client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: m.owner.GetNamespace()}, service)

	// return if already exists
	if err == nil {
		return service, nil
	}
	// something else went wrong
	if !errors.IsNotFound(err) {
		return nil, err
	}

	// create new service
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": m.owner.GetName(),
			},
		},
		Spec: corev1.ServiceSpec{
			// set the labels from the Deployment
			Selector: deployment.Spec.Template.Labels,
			Ports: []corev1.ServicePort{
				{
					Port:       syncthingAPIPort,
					TargetPort: intstr.FromString(targetPort),
					Protocol:   "TCP",
					Name:       "syncthing-api",
				},
			},
		},
	}
	if err = ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
		m.logger.V(3).Error(err, "failed to set owner reference")
		return nil, err
	}
	if err := m.client.Create(ctx, service); err != nil {
		m.logger.Error(err, "error creating the service")
		return nil, err
	}

	return service, nil
}

// ensureDataService Ensures that a service exposing the Syncthing data is present, else it will be created.
// This service allows Syncthing to share data with the rest of the world.
func (m *Mover) ensureDataService(ctx context.Context) (*corev1.Service, error) {
	serviceName := "volsync-" + m.owner.GetName() + "-data"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: m.owner.GetNamespace(),
		},
	}

	err := m.client.Get(ctx, client.ObjectKeyFromObject(service), service)
	if err != nil {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: m.owner.GetNamespace(),
				Labels: map[string]string{
					"app": m.owner.GetName(),
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app": m.owner.GetName(),
				},
				Ports: []corev1.ServicePort{
					{
						Port:       syncthingDataPort,
						TargetPort: intstr.FromInt(syncthingDataPort),
						Protocol:   "TCP",
						Name:       "syncthing-data",
					},
				},
				Type: m.serviceType,
			},
		}

		if err := ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
			m.logger.V(3).Error(err, "failed to set owner reference")
			return nil, err
		}
		if err := m.client.Create(ctx, service); err != nil {
			m.logger.Error(err, "error creating the service")
			return nil, err
		}
	}
	return service, nil
}

// GetDataServiceAddress Will return a string representing the address of the data service, prefixed with TCP.
func (m *Mover) GetDataServiceAddress(service *corev1.Service) (string, error) {
	// format the address based on the type of service we're using
	// supported service types: ClusterIP, LoadBalancer
	address := utils.GetServiceAddress(service)
	if address == "" {
		return "", fmt.Errorf("could not get an address for the service")
	}
	address = "tcp://" + address + ":" + strconv.Itoa(syncthingDataPort)
	return address, nil
}

// Cleanup will remove any resources that were created by the mover.
// This is currently a no-op since Syncthing is always-on.
func (m *Mover) Cleanup(ctx context.Context) (mover.Result, error) {
	err := utils.CleanupObjects(ctx, m.client, m.logger, m.owner, []client.Object{})
	if err != nil {
		return mover.InProgress(), err
	}
	return mover.Complete(), nil
}

// configureSyncthingAPIClient Configures the Syncthing API client if it has not been configured yet.
func (m *Mover) configureSyncthingAPIClient(apiSecret *corev1.Secret) error {
	// set the api key
	m.syncthing.APIConfig.APIKey = string(apiSecret.Data["apikey"])

	// load the TLS certificate
	clientConfig, err := m.loadTLSConfigFromSecret(apiSecret)
	if err != nil {
		return err
	}
	m.syncthing.APIConfig.TLSConfig = clientConfig

	// create a new client or use the existing one
	m.syncthing.APIConfig.Client = m.syncthing.APIConfig.BuildOrUseExistingTLSClient()

	return nil
}

// ensureIsConfigured Makes sure that the Syncthing config is up-to-date.
func (m *Mover) ensureIsConfigured(apiSecret *corev1.Secret) error {
	// This function should make sure to ignore all of devices (leave them as they are) introduced to us by other nodes.
	// This is important because Syncthing will otherwise reconfigure them on its own, creating a cycle where
	// VolSync removes a device, only to be re-added by Syncthing, only to be removed again by VolSync, etc.

	// reconciles the Syncthing object
	err := m.syncthing.FetchLatestInfo()
	if err != nil {
		return err
	}
	m.logger.V(4).Info("Syncthing config", "config", m.syncthing.Config)

	// make sure that the spec doesn't try to add a previously introduced node
	if m.syncthing.PeerListContainsIntroduced(m.peerList) {
		return fmt.Errorf("the peer list contains a node that has been introduced to us by another node")
	}

	// make sure that the spec isn't adding itself as a peer
	if m.syncthing.PeerListContainsSelf(m.peerList) {
		return fmt.Errorf("the peer list contains the node itself")
	}

	// check if the syncthing is configured
	hasChanged := false
	if m.syncthing.NeedsReconfigure(m.peerList) {
		m.logger.V(4).Info("devices need to be reconfigured")
		m.syncthing.UpdateDevices(m.peerList)
		hasChanged = true
	}

	// set the user and password if not already set
	if m.syncthing.Config.GUI.User != string(apiSecret.Data["username"]) ||
		m.syncthing.Config.GUI.Password == "" {
		m.logger.Info("setting user and password")
		m.syncthing.Config.GUI.User = string(apiSecret.Data["username"])
		m.syncthing.Config.GUI.Password = string(apiSecret.Data["password"])
		hasChanged = true
	}

	// update the config
	if hasChanged {
		m.logger.Info("syncthing needs to be updated")
		m.logger.V(4).Info("updating with config", "config", m.syncthing.Config)
		err := m.syncthing.UpdateSyncthingConfig()
		if err != nil {
			m.logger.Error(err, "error updating syncthing config")
			return err
		}
	}
	return nil
}

// ensureStatusIsUpdated Updates the mover's status to be reported by the ReplicationSource object.
func (m *Mover) ensureStatusIsUpdated(dataSVC *corev1.Service) error {
	// get the current status
	err := m.syncthing.FetchLatestInfo()
	if err != nil {
		m.logger.Error(err, "error fetching syncthing status")
		return err
	}

	// fail until we can set the address
	addr, err := m.GetDataServiceAddress(dataSVC)
	if err != nil {
		return err
	}

	// set syncthing-related info
	m.status.Address = addr
	m.status.ID = m.syncthing.SystemStatus.MyID
	m.status.Peers = []volsyncv1alpha1.SyncthingPeerStatus{}

	// add the connected devices to the status
	for deviceID, connectionInfo := range m.syncthing.SystemConnections.Connections {
		// skip our own connection
		if (deviceID == m.syncthing.SystemStatus.MyID) || (deviceID == "") {
			continue
		}

		// obtain the device
		device, ok := m.syncthing.GetDeviceFromID(deviceID)
		if !ok {
			m.logger.Error(fmt.Errorf("could not find device with ID %s", deviceID), "error getting device info")
			continue
		}

		// get the device info
		tcpAddress := AsTCPAddress(connectionInfo.Address)
		introducedBy := device.IntroducedBy
		deviceName := device.Name

		// check connection status
		m.status.Peers = append(m.status.Peers, volsyncv1alpha1.SyncthingPeerStatus{
			ID:           deviceID,
			Address:      tcpAddress,
			Connected:    connectionInfo.Connected,
			Name:         deviceName,
			IntroducedBy: introducedBy,
		})
	}
	return nil
}

// getAPIServiceName Returns the name of the API service exposing the Syncthing API.
func (m *Mover) getAPIServiceName() string {
	serviceName := "volsync-" + m.owner.GetName() + "-api"
	return serviceName
}

// getAPIServiceDNS Returns the DNS of the service exposing the Syncthing API, formatted as ClusterDNS.
func (m *Mover) getAPIServiceDNS() string {
	serviceName := m.getAPIServiceName()
	return fmt.Sprintf("%s.%s", serviceName, m.owner.GetNamespace())
}

// getAPIServiceAddress Returns the ClusterDNS address of the service exposing the Syncthing API.
func (m *Mover) getAPIServiceAddress() string {
	serviceDNS := m.getAPIServiceDNS()
	return fmt.Sprintf("https://%s:%d", serviceDNS, syncthingAPIPort)
}

// loadTLSConfigFromSecret loads the TLS config from the given secret.
func (m *Mover) loadTLSConfigFromSecret(apiSecret *corev1.Secret) (*tls.Config, error) {
	// grab the server cert from the secret
	serverCert, ok := apiSecret.Data["httpsCertPEM"]
	if !ok {
		return nil, fmt.Errorf("could not find the server cert in the secret")
	}

	// create the CA CertPool
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(serverCert)

	// create the TLS config
	conf := &tls.Config{
		// require at least TLS1.2
		MinVersion: tls.VersionTLS12,
		RootCAs:    caCertPool,
	}
	return conf, nil
}
