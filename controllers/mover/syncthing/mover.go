/*
Copyright 2022 The VolSync authors.

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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/mover/syncthing/api"
	"github.com/backube/volsync/controllers/utils"
	"github.com/backube/volsync/controllers/volumehandler"
)

// Environment variables used by the Syncthing image.
const (
	dataDirEnv   = "SYNCTHING_DATA_DIR"
	configDirEnv = "SYNCTHING_CONFIG_DIR"
	certDirEnv   = "SYNCTHING_CERT_DIR"
	apiKeyEnv    = "STGUIAPIKEY"
)

// Directories where files will be loaded into the Syncthing container.
const (
	dataDirMountPath   = "/data"
	configDirMountPath = "/config"
	certDirMountPath   = "/certs"
)

// Volume names loaded by the Deployment.
const (
	certVolumeName   = "https-certs"
	configVolumeName = "syncthing-config"
	dataVolumeName   = "syncthing-data"
)

// Ports used by the Syncthing container.
const (
	apiPort      = 8384
	apiPortName  = "api"
	dataPort     = 22000
	dataPortName = "data"
)

// Key names which are pulled from a Secret by the Syncthing container.
const (
	httpsCertDataKey = "httpsCertPEM"
	httpsKeyDataKey  = "httpsKeyPEM"
	apiKeyDataKey    = "apikey"
	usernameDataKey  = "username"
	passwordDataKey  = "password"
)

// Filepaths for where the HTTPS certificate and key will be
// saved after being loaded into the container.
const (
	httpsKeyPath  = "https-key.pem"
	httpsCertPath = "https-cert.pem"
)

// Miscellaneous constants.
const (
	// configCapacity Sets the size of the config volume used by the Syncthing container.
	configCapacity = "1Gi"
	// resourcePrefix Prefixes every name for resources created by the VolSync controller.
	resourcePrefix = "volsync-"
)

// Mover is the reconciliation logic for the Restic-based data mover.
type Mover struct {
	client              client.Client
	logger              logr.Logger
	owner               client.Object
	eventRecorder       events.EventRecorder
	configCapacity      *resource.Quantity
	configStorageClass  *string
	configAccessModes   []corev1.PersistentVolumeAccessMode
	containerImage      string
	paused              bool
	dataPVCName         *string
	peerList            []volsyncv1alpha1.SyncthingPeer
	status              *volsyncv1alpha1.ReplicationSourceSyncthingStatus
	serviceType         corev1.ServiceType
	syncthingConnection api.SyncthingConnection
	apiConfig           api.APIConfig
}

var _ mover.Mover = &Mover{}

// Name Returns the name of the mover.
func (m *Mover) Name() string { return "syncthing" }

// Synchronize Runs through a synchronization cycle between
// the VolSync operator and the Syncthing data mover.
//
// Synchronize ensures that the necessary resources required by
// the Syncthing Deployment are available, including:
// - a PVC containing the data to be synced
// - PVC for storing configuration data
// - Secret containing the API key, TLS Certificates, and the username/password
//   used to lock down the GUI.
// - Deployment for the Syncthing data mover
// - Service exposing Syncthing's API
// - Service exposing Syncthing's data port
//
// Once the resources are all provided, Synchronize will then
// poll the Syncthing API and make necessary configurations
// based on the data provided to the Syncthing ReplicationSource spec.
//
// Synchronize also updates the ReplicationSource's status
// with information about our local Syncthing instance, as well
// as any connections that have been made to the Syncthing instance.
func (m *Mover) Synchronize(ctx context.Context) (mover.Result, error) {
	dataService, secretAPIKey, err := m.ensureNecessaryResources(ctx)
	if err != nil {
		return mover.InProgress(), err
	}
	if err = m.interactWithSyncthing(dataService, secretAPIKey); err != nil {
		return mover.InProgress(), err
	}
	var retryAfter = 20 * time.Second
	return mover.RetryAfter(retryAfter), nil
}

// ensureNecessaryResources Creates the resources required for VolSync to operate the Syncthing mover,
// and returns references to the data service exposing the Syncthing connection along with
// the secret where necessary credentials are stored.
// If VolSync is unable to ensure the necessary resources, an error is returned.
func (m *Mover) ensureNecessaryResources(ctx context.Context) (*corev1.Service, *corev1.Secret, error) {
	var err error
	dataPVC, err := m.ensureDataPVC(ctx)
	if dataPVC == nil || err != nil {
		return nil, nil, err
	}

	configPVC, err := m.ensureConfigPVC(ctx, dataPVC)
	if configPVC == nil || err != nil {
		return nil, nil, err
	}

	secretAPIKey, err := m.ensureSecretAPIKey(ctx)
	if secretAPIKey == nil || err != nil {
		return nil, nil, err
	}

	sa, err := m.ensureSA(ctx)
	if sa == nil || err != nil {
		return nil, nil, err
	}

	deployment, err := m.ensureDeployment(ctx, dataPVC, configPVC, sa, secretAPIKey)
	if deployment == nil || err != nil {
		return nil, nil, err
	}

	APIService, err := m.ensureAPIService(ctx, deployment)
	if APIService == nil || err != nil {
		return nil, nil, err
	}

	dataService, err := m.ensureDataService(ctx, deployment)
	if dataService == nil || err != nil {
		return nil, nil, err
	}

	return dataService, secretAPIKey, nil
}

// interactWithSyncthing Updates the Syncthing instance with the required connections as defined by VolSync,
// and sets the status of the ReplicationSource to reflect the current state of the Syncthing instance.
// An error is returned when it is unable to do so.
func (m *Mover) interactWithSyncthing(dataService *corev1.Service, apiSecret *corev1.Secret) error {
	// get the API key from the secret
	var err error
	if err = m.validatePeerList(); err != nil {
		return err
	}

	if err = m.configureSyncthingAPIClient(apiSecret); err != nil {
		return err
	}

	// fetch the latest data from Syncthing
	syncthingState, err := m.syncthingConnection.Fetch()
	if err != nil {
		return err
	}

	// configure syncthing before grabbing info & updating status
	if err = m.ensureIsConfigured(apiSecret, syncthingState); err != nil {
		return err
	}

	// obtain the latest state
	if syncthingState, err = m.syncthingConnection.Fetch(); err != nil {
		return err
	}

	if err = m.ensureStatusIsUpdated(dataService, syncthingState); err != nil {
		return err
	}
	return nil
}

// ensureConfigPVC Ensures that there is a PVC persisting Syncthing's config data.
func (m *Mover) ensureConfigPVC(
	ctx context.Context,
	dataPVC *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	// default capacity if none was specified
	var capacity *resource.Quantity = m.configCapacity
	if capacity == nil {
		cap := resource.MustParse(configCapacity)
		capacity = &cap
	}

	options := volsyncv1alpha1.ReplicationSourceVolumeOptions{
		CopyMethod: volsyncv1alpha1.CopyMethodDirect,
		Capacity:   capacity,
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
	configName := resourcePrefix + m.owner.GetName() + "-config"
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

// serviceSelector Returns a mapping of standardized Kubernetes labels.
func (m *Mover) serviceSelector() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      m.owner.GetName(),
		"app.kubernetes.io/component": "syncthing-mover",
		"app.kubernetes.io/part-of":   "volsync",
	}
}

// ensureSecretAPIKey Ensures ensures that the PVC for API secrets either exists or it will create it.
func (m *Mover) ensureSecretAPIKey(ctx context.Context) (*corev1.Secret, error) {
	secretName := resourcePrefix + m.owner.GetName()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: m.owner.GetNamespace(),
		},
	}
	err := m.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)

	// make sure we don't need to do extra work
	if err == nil {
		return secret, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}

	// need to create the secret
	// these will fail only when there is an issue with the OS's RNG
	randomAPIKey, err := GenerateRandomString(32)
	if err != nil {
		return nil, err
	}
	randomPassword, err := GenerateRandomString(32)
	if err != nil {
		return nil, err
	}

	// Generate TLS Certificates for communicating between VolSync and the Syncthing API
	apiServiceDNS := m.getAPIServiceDNS()
	certPEM, certPrivKeyPEM, err := generateTLSCertificatesForSyncthing(apiServiceDNS)
	if err != nil {
		return nil, err
	}

	// create a new secret with the generated values
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		apiKeyDataKey:    []byte(randomAPIKey),
		usernameDataKey:  []byte("syncthing"),
		passwordDataKey:  []byte(randomPassword),
		httpsCertDataKey: certPEM.Bytes(),
		httpsKeyDataKey:  certPrivKeyPEM.Bytes(),
	}

	// ensure secret can be deleted once ReplicationSource is deleted
	if err = ctrl.SetControllerReference(m.owner, secret, m.client.Scheme()); err != nil {
		m.logger.Error(err, "could not set owner ref")
		return nil, err
	}
	utils.SetOwnedByVolSync(secret)
	if err := m.client.Create(ctx, secret); err != nil {
		return nil, err
	}
	m.logger.Info("created secret", secret.Name, secret)
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
	var numReplicas int32 = 1

	deploymentName := resourcePrefix + m.owner.GetName()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: m.owner.GetNamespace(),
			Labels: map[string]string{
				"app": m.owner.GetName(),
			},
		},
	}
	logger := m.logger.WithValues("deployment", client.ObjectKeyFromObject(deployment))

	affinity, err := utils.AffinityFromVolume(ctx, m.client, logger, dataPVC)
	if err != nil {
		logger.Error(err, "unable to determine proper affinity", "PVC", client.ObjectKeyFromObject(dataPVC))
		return nil, err
	}

	// we declare the deployment object in this block line-by-line
	_, err = ctrlutil.CreateOrUpdate(ctx, m.client, deployment, func() error {
		if err := ctrl.SetControllerReference(m.owner, deployment, m.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(deployment)

		deployment.Spec.Replicas = &numReplicas
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: m.serviceSelector(),
		}
		// We don't want >1 ST instance running at a time for a given PVC
		deployment.Spec.Strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}

		deployment.Spec.Template = corev1.PodTemplateSpec{}
		utils.SetOwnedByVolSync(&deployment.Spec.Template)
		deployment.Spec.Template.ObjectMeta.Name = deployment.Name
		utils.AddAllLabels(&deployment.Spec.Template, m.serviceSelector())

		deployment.Spec.Template.Spec.NodeName = affinity.NodeName
		deployment.Spec.Template.Spec.Tolerations = affinity.Tolerations

		deployment.Spec.Template.Spec.ServiceAccountName = sa.Name
		deployment.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
		deployment.Spec.Template.Spec.TerminationGracePeriodSeconds = pointer.Int64(10)
		deployment.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    "syncthing",
				Image:   m.containerImage,
				Command: []string{"/entry.sh"},
				Args:    []string{"run"},
				Env: []corev1.EnvVar{
					{Name: configDirEnv, Value: configDirMountPath},
					{Name: dataDirEnv, Value: dataDirMountPath},
					// tell the mover image where to find the HTTPS certs
					{Name: certDirEnv, Value: certDirMountPath},
					{
						Name: apiKeyEnv,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: apiSecret.Name,
								},
								Key: apiKeyDataKey,
							},
						},
					},
				},
				Ports: []corev1.ContainerPort{
					{Name: apiPortName, ContainerPort: apiPort},
					{Name: dataPortName, ContainerPort: dataPort},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: configVolumeName, MountPath: configDirMountPath},
					{Name: dataVolumeName, MountPath: dataDirMountPath},
					{Name: certVolumeName, MountPath: certDirMountPath},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
				},
			},
		}

		// configure volumes
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
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
				Name: certVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: apiSecret.Name,
						Items: []corev1.KeyToPath{
							{Key: httpsKeyDataKey, Path: httpsKeyPath},
							{Key: httpsCertDataKey, Path: httpsCertPath},
						},
					},
				},
			},
		}
		return nil
	})

	// error from createOrUpdate against a deployment indicates an issue
	if err != nil {
		m.logger.Error(err, "unable to create deployment")
		return nil, err
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
	logger := m.logger.WithValues("service", client.ObjectKeyFromObject(service))

	_, err := ctrlutil.CreateOrUpdate(ctx, m.client, service, func() error {
		if err := ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(service)

		// service should route to the deployment's pods
		service.Spec.Selector = deployment.Spec.Template.Labels
		service.Spec.Ports = []corev1.ServicePort{
			{
				Port:       apiPort,
				TargetPort: intstr.FromString(targetPort),
				Protocol:   "TCP",
				Name:       apiPortName,
			},
		}
		return nil
	})

	// return service XOR error
	if err != nil {
		return nil, err
	}
	return service, nil
}

// ensureDataService Ensures that a service exposing the Syncthing data is present, else it will be created.
// This service allows Syncthing to share data with the rest of the world.
func (m *Mover) ensureDataService(ctx context.Context, deployment *appsv1.Deployment) (*corev1.Service, error) {
	serviceName := resourcePrefix + m.owner.GetName() + "-data"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: m.owner.GetNamespace(),
		},
	}

	logger := m.logger.WithValues("service", client.ObjectKeyFromObject(service))
	_, err := ctrl.CreateOrUpdate(ctx, m.client, service, func() error {
		if err := ctrl.SetControllerReference(m.owner, service, m.client.Scheme()); err != nil {
			logger.Error(err, utils.ErrUnableToSetControllerRef)
			return err
		}
		utils.SetOwnedByVolSync(service)

		service.Spec.Type = m.serviceType
		service.Spec.Selector = deployment.Spec.Template.Labels
		service.Spec.Ports = []corev1.ServicePort{
			{
				Port:       dataPort,
				TargetPort: intstr.FromInt(dataPort),
				// This port prefers TCP but may also be UDP
				// this may benefit some envs w/ non-TCP connections
				Protocol: "TCP",
				Name:     dataPortName,
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
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
	address = asTCPAddress(address + ":" + strconv.Itoa(dataPort))
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
func (m *Mover) configureSyncthingAPIClient(
	apiSecret *corev1.Secret,
) error {
	// if the API URL has not already been overridden, we will set the
	// API URL here
	if m.apiConfig.APIURL == "" {
		// get the API URL
		m.apiConfig.APIURL = m.getAPIServiceAddress()
	}

	// configure authentication per request
	m.apiConfig.APIKey = string(apiSecret.Data[apiKeyDataKey])
	clientConfig, err := m.loadTLSConfigFromSecret(apiSecret)
	if err != nil {
		return err
	}
	m.apiConfig.TLSConfig = clientConfig

	// create a new client or use the existing one
	m.apiConfig.Client = m.apiConfig.TLSClient()
	m.syncthingConnection = api.NewConnection(
		m.apiConfig,
		m.logger.WithName("syncthingConnection").V(4),
	)

	return err
}

// validatePeerList Checks to make sure that there are no duplicate entries within the provided peerList,
// and errors if there are.
func (m *Mover) validatePeerList() error {
	uniquePeers := make(map[string]bool)
	for _, peer := range m.peerList {
		if _, ok := uniquePeers[peer.ID]; ok {
			return fmt.Errorf("duplicate peer found in peer list: %s", peer.ID)
		}
		uniquePeers[peer.ID] = true
	}
	return nil
}

// ensureIsConfigured Takes the given syncthing state and updates it with the necessary information
// from the peerList as well as the given apiSecret. An error is returned when we are unsuccessful in
// updating the configuration.
//
// If there is no User/Password set on the object, or a user is set but doesn't match the value in the secret,
// then ensureIsConfigured will update the Syncthing state to match the values in the secret.
func (m *Mover) ensureIsConfigured(apiSecret *corev1.Secret, syncthing *api.Syncthing) error {
	// nil check
	if apiSecret == nil || syncthing == nil {
		return fmt.Errorf("arguments cannot be nil")
	}

	m.logger.V(4).Info("Syncthing config", "config", syncthing.Configuration)

	// make sure that the spec isn't adding itself as a peer
	for _, peer := range m.peerList {
		if peer.ID == syncthing.MyID() {
			return fmt.Errorf("the peer list contains the node itself")
		}
	}

	// check if the syncthing is configured
	hasChanged := false
	if syncthingNeedsReconfigure(m.peerList, syncthing) {
		m.logger.V(4).Info("devices need to be reconfigured")
		// configure the syncthing state with the new devices
		if err := updateSyncthingDevices(m.peerList, syncthing); err != nil {
			return err
		}
		hasChanged = true
	}

	// set the user and password if not already set
	if syncthing.Configuration.GUI.User != string(apiSecret.Data[usernameDataKey]) ||
		syncthing.Configuration.GUI.Password == "" {
		m.logger.Info("setting user and password")
		syncthing.Configuration.GUI.User = string(apiSecret.Data[usernameDataKey])
		syncthing.Configuration.GUI.Password = string(apiSecret.Data[passwordDataKey])
		hasChanged = true
	}

	// update the config
	if hasChanged {
		// get syncthing object & update the remote config w/ it
		m.logger.Info("syncthing needs to be updated")
		m.logger.V(4).Info("updating with config", "config", syncthing.Configuration)
		err := m.syncthingConnection.PublishConfig(syncthing.Configuration)
		if err != nil {
			m.logger.Error(err, "error updating syncthing config")
			return err
		}
	}
	return nil
}

// ensureStatusIsUpdated Updates the mover's status to be reported by the ReplicationSource object.
func (m *Mover) ensureStatusIsUpdated(dataSVC *corev1.Service,
	syncthing *api.Syncthing) error {
	// fail until we can get the address
	addr, err := m.GetDataServiceAddress(dataSVC)
	if err != nil {
		return err
	}

	// set syncthing-related info
	m.status.Address = asTCPAddress(addr)
	m.status.ID = syncthing.MyID()
	m.status.Peers = m.getConnectedPeers(syncthing)

	return nil
}

// getConnectedPeers Retrieves a list of all the peers connected to our Syncthing instance.
func (m *Mover) getConnectedPeers(syncthing *api.Syncthing) []volsyncv1alpha1.SyncthingPeerStatus {
	connectedPeers := []volsyncv1alpha1.SyncthingPeerStatus{}

	// add the connected devices to the status
	for deviceID, connectionInfo := range syncthing.SystemConnections.Connections {
		// skip our own connection
		if (deviceID == syncthing.MyID()) || (deviceID == "") {
			continue
		}

		// obtain the device
		device, ok := syncthing.GetDeviceFromID(deviceID)
		if !ok {
			m.logger.Error(fmt.Errorf("could not find device with ID %s", deviceID), "error getting device info")
			continue
		}

		// Set the device information.
		// Syncthing has support for UDP, however the only available connection types are:
		// - TCP (client)
		// - TCP (server)
		// - Relay (client)
		// - Relay (server)
		// We are not using relays in VolSync (yet), so the only possible connections are TCP.
		// Therefore we must format the connection information as a TCP address.
		// See:
		//  - https://docs.syncthing.net/rest/system-connections-get.html
		//  - https://forum.syncthing.net/t/specifying-protocols-without-global-announce-or-relay/18565
		tcpAddress := asTCPAddress(connectionInfo.Address)
		introducedBy := device.IntroducedBy
		deviceName := device.Name

		// check connection status
		connectedPeers = append(connectedPeers, volsyncv1alpha1.SyncthingPeerStatus{
			ID:           deviceID,
			Address:      tcpAddress,
			Connected:    connectionInfo.Connected,
			Name:         deviceName,
			IntroducedBy: introducedBy.GoString(),
		})
	}
	return connectedPeers
}

// getAPIServiceName Returns the name of the API service exposing the Syncthing API.
func (m *Mover) getAPIServiceName() string {
	serviceName := resourcePrefix + m.owner.GetName() + "-api"
	return serviceName
}

// getAPIServiceDNS Returns the DNS of the service exposing the Syncthing API, formatted as ClusterDNS.
func (m *Mover) getAPIServiceDNS() string {
	serviceName := m.getAPIServiceName()
	return fmt.Sprintf("%s.%s", serviceName, m.owner.GetNamespace())
}

// getAPIServiceAddress Returns a ClusterDNS address of the service exposing the Syncthing API.
func (m *Mover) getAPIServiceAddress() string {
	serviceDNS := m.getAPIServiceDNS()
	return fmt.Sprintf("https://%s:%d", serviceDNS, apiPort)
}

// loadTLSConfigFromSecret loads the TLS config from the given secret.
func (m *Mover) loadTLSConfigFromSecret(apiSecret *corev1.Secret) (*tls.Config, error) {
	// grab the server cert from the secret
	serverCert, ok := apiSecret.Data[httpsCertDataKey]
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
