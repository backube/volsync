/*
Copyright 2026 The VolSync authors.

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

package platform

import (
	"context"
	"fmt"

	"crypto/tls"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ocpconfigv1 "github.com/openshift/api/config/v1"
	ocptls "github.com/openshift/controller-runtime-common/pkg/tls"
)

func getTLSProfile(ctx context.Context, k8sClient client.Client,
	logger logr.Logger) (*ocpconfigv1.TLSProfileSpec, error) {
	// Fetch the TLS profile from the APIServer resource.
	logger.Info("Cluster is OpenShift, querying API server for TLSProfiles")
	tlsSecurityProfileSpec, err := ocptls.FetchAPIServerTLSProfile(ctx, k8sClient)
	if err != nil {
		logger.Error(err, "unable to get TLS profile from API server")
		return nil, err
	}
	logger.Info("Using TLS profile",
		"minTLSVersion", tlsSecurityProfileSpec.MinTLSVersion,
		"ciphers", tlsSecurityProfileSpec.Ciphers)

	return &tlsSecurityProfileSpec, nil
}

func GetTLSProfileIfOpenShift(ctx context.Context, k8sClient client.Client,
	logger logr.Logger) (*ocpconfigv1.TLSProfileSpec, error) {
	p, err := GetProperties(ctx, k8sClient, logger)
	if err != nil {
		return nil, err
	}
	if !p.IsOpenShift {
		return nil, nil // Not OpenShift, nothing to do here
	}
	return p.TLSSecurityProfileSpec, nil
}

func GetTLSConfigFromProfile(tlsSecurityProfileSpec ocpconfigv1.TLSProfileSpec, logger logr.Logger) func(*tls.Config) {
	// Create the TLS configuration function for the server endpoints.
	tlsConfig, unsupportedCiphers := ocptls.NewTLSConfigFromProfile(tlsSecurityProfileSpec)
	if len(unsupportedCiphers) > 0 {
		logger.Info("TLS configuration contains unsupported ciphers that will be ignored",
			"unsupportedCiphers", unsupportedCiphers)
	}

	return tlsConfig
}

// Setup TLS Security Profile Watcher to monitor for TLS profile changes.
// When the cluster's TLS profile changes, cancelFunc() will be called.
// This can be used to initiate shutdown of the operator to restart
// and pickup the changes.
func InitTLSSecurityProfileWatcherWithManager(mgr manager.Manager,
	initialTLSProfileSpec ocpconfigv1.TLSProfileSpec, logger logr.Logger, cancelFunc func()) error {
	tlsProfileWatcher := &ocptls.SecurityProfileWatcher{
		Client:                mgr.GetClient(),
		InitialTLSProfileSpec: initialTLSProfileSpec,
		OnProfileChange: func(_ context.Context, oldProfile, newProfile ocpconfigv1.TLSProfileSpec) {
			logger.Info("TLS security profile has changed, initiating graceful shutdown to reload configuration",
				"oldMinTLSVersion", oldProfile.MinTLSVersion,
				"newMinTLSVersion", newProfile.MinTLSVersion,
				"oldCiphers", oldProfile.Ciphers,
				"newCiphers", newProfile.Ciphers)
			// Cancel the context to trigger a graceful shutdown of the manager.
			// The operator will be restarted by the deployment controller.
			cancelFunc()
		},
	}

	return tlsProfileWatcher.SetupWithManager(mgr)
}

// Parse string version of ocpconfigv1.TLSProtocolVersion in format that others (such as stunnel) can interpret
func ParseTLSVersion(version ocpconfigv1.TLSProtocolVersion) (string, error) {
	switch version {
	case ocpconfigv1.VersionTLS10:
		return "TLSv1", nil // Note: it looks like some other places may use "TLSv1.0"
	case ocpconfigv1.VersionTLS11:
		return "TLSv1.1", nil
	case ocpconfigv1.VersionTLS12:
		return "TLSv1.2", nil
	case ocpconfigv1.VersionTLS13:
		return "TLSv1.3", nil
	default:
		return "", fmt.Errorf("unknown TLS version: %s", version)
	}
}

func ParseTLS13CipherSuitesForStunnelPSK(tlsSecurityProfileSpec ocpconfigv1.TLSProfileSpec) string {
	cipherSuites := ""
	for _, cipher := range tlsSecurityProfileSpec.Ciphers {
		// Stunnel "ciphersuites" in the stunnel.conf are for TLS 1.3 only
		// only allow these specific ciphers which are supported by stunnel with TLS 1.3 and work with PSK
		// This means if a user specifies only invalid ciphers, we will just allow the default (i.e. return "")
		if cipher != "TLS_AES_128_GCM_SHA256" && cipher != "TLS_CHACHA20_POLY1305_SHA256" {
			// Note: not including TLS_AES_256_GCM_SHA384 right now as it doesn't appear to work with TLS 1.3 & PSK
			continue
		}
		if cipherSuites == "" {
			cipherSuites = cipher
		} else {
			cipherSuites = cipherSuites + ":" + cipher
		}
	}
	return cipherSuites
}
