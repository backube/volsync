package api

import (
	"github.com/go-logr/logr"
	"github.com/syncthing/syncthing/lib/config"
)

// Fetch Pulls all of Syncthing's latest information from the API and stores it
// in the object's local storage.
func (s *syncthingAPIConnection) Fetch() (*Syncthing, error) {
	// get & store config
	conf, err := s.fetchConfig()
	if err != nil {
		return nil, err
	}

	// get & store connection info
	systemConnections, err := s.fetchSystemConnections()
	if err != nil {
		return nil, err
	}

	// get and store system status
	systemStatus, err := s.fetchSystemStatus()
	if err != nil {
		return nil, err
	}

	return &Syncthing{
		Configuration:     *conf,
		SystemConnections: *systemConnections,
		SystemStatus:      *systemStatus,
	}, nil
}

// PublishConfig Updates the Syncthing API with the stored configuration data.
// An error is returned in the case of a failure.
func (s *syncthingAPIConnection) PublishConfig(conf config.Configuration) error {
	// update the config
	s.logger.Info("Updating Syncthing config")
	_, err := s.jsonRequest("/rest/config", "PUT", conf)
	if err != nil {
		s.logger.Error(err, "Failed to update Syncthing config")
	}
	return err
}

// NewConnection accepts an APIConfig object and a logger and creates a SyncthingConnection
// object in return.
func NewConnection(cfg APIConfig, logger logr.Logger) SyncthingConnection {
	return &syncthingAPIConnection{
		apiConfig: cfg,
		logger:    logger,
	}
}
