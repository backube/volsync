package api

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

// Headers Returns a map containing the necessary headers for Syncthing API requests.
// When no API Key is provided, an error is returned.
func (api *APIConfig) Headers() (map[string]string, error) {
	if api.APIKey == "" {
		return nil, errors.New("API Key is not set")
	}

	return map[string]string{
		"X-API-Key":    api.APIKey,
		"Content-Type": "application/json",
	}, nil
}

// BuildTLSClient Returns a new TLS client for Syncthing API requests.
func (api APIConfig) BuildOrUseExistingTLSClient() *http.Client {
	if api.Client != nil {
		return api.Client
	}
	return BuildTLSClient(api.TLSConfig)
}

// BuildTLSClient Creates a new TLS client for Syncthing API requests using the given tlsConfig,
// or its own if one isn't provided.
func BuildTLSClient(tlsConfig *tls.Config) *http.Client {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// load the TLS config with certificates
	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 5,
	}
	return client
}
