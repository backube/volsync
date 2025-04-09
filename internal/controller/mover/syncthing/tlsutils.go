//go:build !disable_syncthing

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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

const (
	OrganizationName = "Backube"
	OrganizationUnit = "VolSync"
)

// createOrganizationCertificate Returns an x.509 certificate with an expiry period of
// 10 years. The certificate will use the OrganizationName and OrganizationUnit constants
// as defined in this package. This function also sets the extended key usage to allow
// client authentication and server authentication.
//
// An error is returned in the event that we cannot
// generate the serial number.
func createOrganizationCertificate() (*x509.Certificate, error) {
	// generate a bunch of random bytes
	serialNumber, err := GenerateRandomBytes(20)
	if err != nil {
		return nil, err
	}

	// expire in 10 years
	notBefore := time.Now()
	notAfter := notBefore.AddDate(10, 0, 0)

	// convert the serial number to a bigint
	serialNumberBigInt := new(big.Int).SetBytes(serialNumber)

	// set up our certificate
	cert := &x509.Certificate{}
	cert.SerialNumber = serialNumberBigInt
	cert.NotBefore = notBefore
	cert.NotAfter = notAfter
	cert.Subject.Organization = []string{OrganizationName}
	cert.Subject.OrganizationalUnit = []string{OrganizationUnit}
	cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}

	return cert, nil
}

// generateCACertificate Creates a Root X.509 Certificate to be consumed by Syncthing.
func generateCACertificate() (*x509.Certificate, error) {
	ca, err := createOrganizationCertificate()
	if err != nil {
		return nil, err
	}

	// set up our CA certificate
	ca.IsCA = true
	ca.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
	ca.BasicConstraintsValid = true
	return ca, nil
}

// generateSyncthingCertificate Generates a Certificate and a Private/Public Key pair to sign the PEM.
func generateSyncthingTLSCertificate(DNSNames []string) (*x509.Certificate, error) {
	cert, err := createOrganizationCertificate()
	if err != nil {
		return nil, err
	}

	// set up our server certificate
	cert.DNSNames = DNSNames
	cert.KeyUsage = x509.KeyUsageDigitalSignature

	return cert, nil
}

// getCertificatePEMs Accepts a certificate and RSA Private Key
// and returns their PEM encodings respectively, or an error if there was
// an issue with the encodings.
func getCertificatePEMs(certBytes []byte, certPrivKey *rsa.PrivateKey) (*bytes.Buffer, *bytes.Buffer, error) {
	if certPrivKey == nil || certBytes == nil {
		return nil, nil, fmt.Errorf("arguments cannot be nil")
	}

	certPEM := new(bytes.Buffer)
	err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return nil, nil, err
	}
	certPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	if err != nil {
		return nil, nil, err
	}
	return certPEM, certPrivKeyPEM, nil
}

// generateTLSCertificatesForSyncthing generates a self-signed PEM-encoded certificate and key for Syncthing
// which the VolSync client and Syncthing API Server will use to communicate with each other.
func generateTLSCertificatesForSyncthing(
	APIServiceAddress string,
) (*bytes.Buffer, *bytes.Buffer, error) {
	// we will need to perform checks if the apiServiceDNS has changed
	// and re-generate in case the TLS Certificates have changed

	// generate the Syncthing TLS certificate
	cert, err := generateSyncthingTLSCertificate([]string{APIServiceAddress})
	if err != nil {
		return nil, nil, err
	}

	// generate a private key for the certificate
	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// create a Root CA for our certificate
	// serial number should be the current time in unix epoch time
	ca, err := generateCACertificate()
	if err != nil {
		return nil, nil, err
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM, certPrivKeyPEM, err := getCertificatePEMs(certBytes, certPrivKey)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, certPrivKeyPEM, nil
}
