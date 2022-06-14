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
