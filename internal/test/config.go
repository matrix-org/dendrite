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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"gopkg.in/yaml.v2"
)

const (
	// ConfigFile is the name of the config file for a server.
	ConfigFile = "dendrite.yaml"
	// ServerKeyFile is the name of the file holding the matrix server private key.
	ServerKeyFile = "server_key.pem"
	// TLSCertFile is the name of the file holding the TLS certificate used for federation.
	TLSCertFile = "tls_cert.pem"
	// TLSKeyFile is the name of the file holding the TLS key used for federation.
	TLSKeyFile = "tls_key.pem"
	// MediaDir is the name of the directory used to store media.
	MediaDir = "media"
)

// MakeConfig makes a config suitable for running integration tests.
// Generates new matrix and TLS keys for the server.
func MakeConfig(configDir, kafkaURI, database, host string, startPort int) (*config.Dendrite, int, error) {
	var cfg config.Dendrite
	cfg.Defaults(true)

	port := startPort
	assignAddress := func() config.HTTPAddress {
		result := config.HTTPAddress(fmt.Sprintf("http://%s:%d", host, port))
		port++
		return result
	}

	serverKeyPath := filepath.Join(configDir, ServerKeyFile)
	tlsCertPath := filepath.Join(configDir, TLSKeyFile)
	tlsKeyPath := filepath.Join(configDir, TLSCertFile)
	mediaBasePath := filepath.Join(configDir, MediaDir)

	if err := NewMatrixKey(serverKeyPath); err != nil {
		return nil, 0, err
	}

	if err := NewTLSKey(tlsKeyPath, tlsCertPath); err != nil {
		return nil, 0, err
	}

	cfg.Version = config.Version

	cfg.Global.ServerName = gomatrixserverlib.ServerName(assignAddress())
	cfg.Global.PrivateKeyPath = config.Path(serverKeyPath)

	cfg.MediaAPI.BasePath = config.Path(mediaBasePath)

	cfg.Global.JetStream.Addresses = []string{kafkaURI}

	// TODO: Use different databases for the different schemas.
	// Using the same database for every schema currently works because
	// the table names are globally unique. But we might not want to
	// rely on that in the future.
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(database)
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(database)
	cfg.KeyServer.Database.ConnectionString = config.DataSource(database)
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(database)
	cfg.RoomServer.Database.ConnectionString = config.DataSource(database)
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(database)
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(database)

	cfg.AppServiceAPI.InternalAPI.Listen = assignAddress()
	cfg.FederationAPI.InternalAPI.Listen = assignAddress()
	cfg.KeyServer.InternalAPI.Listen = assignAddress()
	cfg.MediaAPI.InternalAPI.Listen = assignAddress()
	cfg.RoomServer.InternalAPI.Listen = assignAddress()
	cfg.SyncAPI.InternalAPI.Listen = assignAddress()
	cfg.UserAPI.InternalAPI.Listen = assignAddress()

	cfg.AppServiceAPI.InternalAPI.Connect = cfg.AppServiceAPI.InternalAPI.Listen
	cfg.FederationAPI.InternalAPI.Connect = cfg.FederationAPI.InternalAPI.Listen
	cfg.KeyServer.InternalAPI.Connect = cfg.KeyServer.InternalAPI.Listen
	cfg.MediaAPI.InternalAPI.Connect = cfg.MediaAPI.InternalAPI.Listen
	cfg.RoomServer.InternalAPI.Connect = cfg.RoomServer.InternalAPI.Listen
	cfg.SyncAPI.InternalAPI.Connect = cfg.SyncAPI.InternalAPI.Listen
	cfg.UserAPI.InternalAPI.Connect = cfg.UserAPI.InternalAPI.Listen

	return &cfg, port, nil
}

// WriteConfig writes the config file to the directory.
func WriteConfig(cfg *config.Dendrite, configDir string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(configDir, ConfigFile), data, 0666)
}

// NewMatrixKey generates a new ed25519 matrix server key and writes it to a file.
func NewMatrixKey(matrixKeyPath string) (err error) {
	var data [35]byte
	_, err = rand.Read(data[:])
	if err != nil {
		return err
	}
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
		Bytes: data[3:],
	})
	return err
}

const certificateDuration = time.Hour * 24 * 365 * 10

func generateTLSTemplate(dnsNames []string) (*rsa.PrivateKey, *x509.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
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
func NewTLSKey(tlsKeyPath, tlsCertPath string) error {
	priv, template, err := generateTLSTemplate(nil)
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

func NewTLSKeyWithAuthority(serverName, tlsKeyPath, tlsCertPath, authorityKeyPath, authorityCertPath string) error {
	priv, template, err := generateTLSTemplate([]string{serverName})
	if err != nil {
		return err
	}

	// load the authority key
	dat, err := ioutil.ReadFile(authorityKeyPath)
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
	dat, err = ioutil.ReadFile(authorityCertPath)
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
