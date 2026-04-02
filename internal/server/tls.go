package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// LoadOrGenerateTLSConfig loads TLS certs from the given paths, or generates
// a self-signed certificate if they don't exist.
func LoadOrGenerateTLSConfig(certPath, keyPath, dataDir string) (*tls.Config, error) {
	// If explicit paths given and they exist, use them
	if certPath != "" && keyPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("loading TLS cert/key: %w", err)
		}
		return hardenedTLSConfig(cert), nil
	}

	// Try loading from data dir
	defaultCert := filepath.Join(dataDir, "server.crt")
	defaultKey := filepath.Join(dataDir, "server-tls.key")

	if _, err := os.Stat(defaultCert); err == nil {
		if _, err := os.Stat(defaultKey); err == nil {
			cert, err := tls.LoadX509KeyPair(defaultCert, defaultKey)
			if err != nil {
				return nil, fmt.Errorf("loading existing TLS cert: %w", err)
			}
			return hardenedTLSConfig(cert), nil
		}
	}

	// Generate self-signed cert
	cert, err := generateSelfSignedCert(defaultCert, defaultKey)
	if err != nil {
		return nil, fmt.Errorf("generating self-signed cert: %w", err)
	}
	return hardenedTLSConfig(cert), nil
}

func hardenedTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

func generateSelfSignedCert(certPath, keyPath string) (tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Enclave"},
			CommonName:   "Enclave Server",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // 1 year

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		// Allow connections from any local IP
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
		DNSNames: []string{"localhost", "enclave.local"},
	}

	// Add all local IPs
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				template.IPAddresses = append(template.IPAddresses, ipnet.IP)
			}
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Write cert file
	certFile, err := os.Create(certPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certFile.Close()

	// Write key file
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return tls.Certificate{}, err
	}
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyFile.Close()

	return tls.LoadX509KeyPair(certPath, keyPath)
}
