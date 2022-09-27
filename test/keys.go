// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const (
	// ServerKeyFile is the name of the file holding the matrix server private key.
	ServerKeyFile = "server_key.pem"
	// TLSCertFile is the name of the file holding the TLS certificate used for federation.
	TLSCertFile = "tls_cert.pem"
	// TLSKeyFile is the name of the file holding the TLS key used for federation.
	TLSKeyFile = "tls_key.pem"
)

// NewMatrixKey generates a new ed25519 matrix server key and writes it to a file.
func NewMatrixKey(matrixKeyPath string) (err error) {
	var data [35]byte
	_, err = rand.Read(data[:])
	if err != nil {
		return err
	}
	return SaveMatrixKey(matrixKeyPath, data[3:])
}

func SaveMatrixKey(matrixKeyPath string, data ed25519.PrivateKey) error {
	keyOut, err := os.OpenFile(matrixKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	defer (func() {
		err = keyOut.Close()
	})()

	keyID := base64.RawURLEncoding.EncodeToString(data[:])
	keyID = strings.ReplaceAll(keyID, "-", "")
	keyID = strings.ReplaceAll(keyID, "_", "")

	err = pem.Encode(keyOut, &pem.Block{
		Type: "MATRIX PRIVATE KEY",
		Headers: map[string]string{
			"Key-ID": fmt.Sprintf("ed25519:%s", keyID[:6]),
		},
		Bytes: data,
	})
	return err
}

const certificateDuration = time.Hour * 24 * 365 * 10

func generateTLSTemplate(dnsNames []string, bitSize int) (*rsa.PrivateKey, *x509.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(certificateDuration)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}
	return priv, &template, nil
}

func writeCertificate(tlsCertPath string, derBytes []byte) error {
	certOut, err := os.Create(tlsCertPath)
	if err != nil {
		return err
	}
	defer certOut.Close() // nolint: errcheck
	return pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func writePrivateKey(tlsKeyPath string, priv *rsa.PrivateKey) error {
	keyOut, err := os.OpenFile(tlsKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close() // nolint: errcheck
	err = pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	return err
}

// NewTLSKey generates a new RSA TLS key and certificate and writes it to a file.
func NewTLSKey(tlsKeyPath, tlsCertPath string, keySize int) error {
	priv, template, err := generateTLSTemplate(nil, keySize)
	if err != nil {
		return err
	}

	// Self-signed certificate: template == parent
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	if err = writeCertificate(tlsCertPath, derBytes); err != nil {
		return err
	}
	return writePrivateKey(tlsKeyPath, priv)
}

func NewTLSKeyWithAuthority(serverName, tlsKeyPath, tlsCertPath, authorityKeyPath, authorityCertPath string, keySize int) error {
	priv, template, err := generateTLSTemplate([]string{serverName}, keySize)
	if err != nil {
		return err
	}

	// load the authority key
	dat, err := os.ReadFile(authorityKeyPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode([]byte(dat))
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return errors.New("authority .key is not a valid pem encoded rsa private key")
	}
	authorityPriv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}

	// load the authority certificate
	dat, err = os.ReadFile(authorityCertPath)
	if err != nil {
		return err
	}
	block, _ = pem.Decode([]byte(dat))
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("authority .crt is not a valid pem encoded x509 cert")
	}
	var caCerts []*x509.Certificate
	caCerts, err = x509.ParseCertificates(block.Bytes)
	if err != nil {
		return err
	}
	if len(caCerts) != 1 {
		return errors.New("authority .crt contains none or more than one cert")
	}
	authorityCert := caCerts[0]

	// Sign the new certificate using the authority's key/cert
	derBytes, err := x509.CreateCertificate(rand.Reader, template, authorityCert, &priv.PublicKey, authorityPriv)
	if err != nil {
		return err
	}

	if err = writeCertificate(tlsCertPath, derBytes); err != nil {
		return err
	}
	return writePrivateKey(tlsKeyPath, priv)
}
