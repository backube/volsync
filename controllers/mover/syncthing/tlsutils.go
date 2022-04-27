package syncthing

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

const (
	OrganizationName = "Backube"
	OrganizationUnit = "VolSync"
)

// GetCACertificate Creates a Root X.509 Certificate to be consumed by Syncthing.
func GetCACertificate() (*x509.Certificate, error) {
	// generate a bunch of random bytes
	serialNumber, err := GenerateRandomBytes(2048)
	if err != nil {
		return nil, err
	}

	// setup expiry dates
	notBefore := time.Now()
	// expire in 10 years
	notAfter := notBefore.AddDate(10, 0, 0)

	// convert the serial number to a bigint
	serialNumberBigInt := new(big.Int).SetBytes(serialNumber)

	// set up our CA certificate
	ca := &x509.Certificate{
		SerialNumber: serialNumberBigInt,
		Subject: pkix.Name{
			Organization:       []string{OrganizationName},
			OrganizationalUnit: []string{OrganizationUnit},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	return ca, nil
}

// GenerateSyncthingRootCA Generates a CA Certificate and a Private/Public Key pair to sign the PEM.
func GenerateSyncthingRootCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	// serial number should be the current time in unix epoch time
	ca, err := GetCACertificate()
	if err != nil {
		return nil, nil, err
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	// pem encode
	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	caPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	if err != nil {
		return nil, nil, err
	}

	return ca, caPrivKey, nil
}

// GenerateSyncthingCertificate Generates a Certificate and a Private/Public Key pair to sign the PEM.
func GenerateSyncthingTLSCertificate(DNSNames []string, IPAddresses []net.IP) (*x509.Certificate, error) {
	// generate another random serial number
	serialNumber, err := GenerateRandomBytes(2048)
	if err != nil {
		return nil, err
	}

	// setup expiry dates
	notBefore := time.Now()
	// expire in 10 years
	notAfter := notBefore.AddDate(10, 0, 0)

	// convert the serial number to a bigint
	serialNumberBigInt := new(big.Int).SetBytes(serialNumber)

	// set up our server certificate
	cert := &x509.Certificate{
		SerialNumber: serialNumberBigInt,
		Subject: pkix.Name{
			Organization:       []string{OrganizationName},
			OrganizationalUnit: []string{OrganizationUnit},
		},
		DNSNames:    DNSNames,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	return cert, nil
}

// GenerateTLSCertificatesForSyncthing generates a self-signed PEM-encoded certificate and key for Syncthing
// which the VolSync client and Syncthing API Server will use to communicate with each other.
func GenerateTLSCertificatesForSyncthing(
	APIServiceAddress string,
) (*bytes.Buffer, *bytes.Buffer, error) {
	// we will need to perform checks if the apiServiceDNS has changed
	// and re-generate in case the TLS Certificates have changed

	// generate the Syncthing TLS certificate
	cert, err := GenerateSyncthingTLSCertificate([]string{APIServiceAddress}, []net.IP{net.ParseIP("127.0.0.1")})
	if err != nil {
		return nil, nil, err
	}

	// generate a private key for the certificate
	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// create a Root CA for our certificate
	ca, caPrivKey, err := GenerateSyncthingRootCA()
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(certPEM, &pem.Block{
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
