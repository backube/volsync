package api

import (
	"crypto/tls"
	"net/http"
	"time"
)

// TLSClient Returns a TLS Client used by the API Config.
// If the client field is nil, then a new TLS Client is built using
// either the custom TLS Config set or a default tlsConfig with version 1.2
func (api APIConfig) TLSClient() *http.Client {
	if api.Client != nil {
		return api.Client
	}
	return buildTLSClient(api.TLSConfig)
}

// buildTLSClient Creates a new TLS client for Syncthing API requests using the given tlsConfig,
// or its own if one isn't provided.
func buildTLSClient(tlsConfig *tls.Config) *http.Client {
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
