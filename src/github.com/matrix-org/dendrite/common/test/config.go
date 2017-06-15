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
	"fmt"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"time"
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

	port := startPort
	assignAddress := func() config.Address {
		result := config.Address(fmt.Sprintf("%s:%d", host, port))
		port++
		return result
	}

	serverKeyPath := filepath.Join(configDir, ServerKeyFile)
	tlsCertPath := filepath.Join(configDir, TLSKeyFile)
	tlsKeyPath := filepath.Join(configDir, TLSCertFile)
	mediaBasePath := filepath.Join(configDir, MediaDir)

	if err := newMatrixKey(serverKeyPath); err != nil {
		return nil, 0, err
	}

	if err := newTLSKey(tlsKeyPath, tlsCertPath); err != nil {
		return nil, 0, err
	}

	cfg.Version = config.Version

	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(assignAddress())
	cfg.Matrix.PrivateKeyPath = config.Path(serverKeyPath)
	cfg.Matrix.FederationCertificatePaths = []config.Path{config.Path(tlsCertPath)}

	cfg.Media.BasePath = config.Path(mediaBasePath)

	cfg.Kafka.Addresses = []string{kafkaURI}
	// TODO: Different servers should be using different topics.
	// Make this configurable somehow?
	cfg.Kafka.Topics.InputRoomEvent = "test.room.input"
	cfg.Kafka.Topics.OutputRoomEvent = "test.room.output"

	// TODO: Use different databases for the different schemas.
	// Using the same database for every schema currently works because
	// the table names are globally unique. But we might not want to
	// rely on that in the future.
	cfg.Database.Account = config.DataSource(database)
	cfg.Database.Device = config.DataSource(database)
	cfg.Database.MediaServer = config.DataSource(database)
	cfg.Database.RoomServer = config.DataSource(database)
	cfg.Database.ServerKey = config.DataSource(database)
	cfg.Database.SyncServer = config.DataSource(database)

	cfg.Listen.ClientAPI = assignAddress()
	cfg.Listen.FederationAPI = assignAddress()
	cfg.Listen.MediaAPI = assignAddress()
	cfg.Listen.RoomServer = assignAddress()
	cfg.Listen.SyncAPI = assignAddress()

	return &cfg, port, nil
}

// WriteConfig writes the config file to the directory.
func WriteConfig(cfg *config.Dendrite, configDir string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(filepath.Join(configDir, ConfigFile), data, 0666); err != nil {
		return err
	}
	return nil
}

// newMatrixKey generates a new ed25519 matrix server key and writes it to a file.
func newMatrixKey(matrixKeyPath string) error {
	var data [35]byte
	if _, err := rand.Read(data[:]); err != nil {
		return err
	}
	keyOut, err := os.OpenFile(matrixKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	if err = pem.Encode(keyOut, &pem.Block{
		Type: "MATRIX PRIVATE KEY",
		Headers: map[string]string{
			"Key-ID": "ed25519:" + base64.RawStdEncoding.EncodeToString(data[:3]),
		},
		Bytes: data[3:],
	}); err != nil {
		return err
	}

	return nil
}

const certificateDuration = time.Hour * 24 * 365 * 10

// newTLSKey generates a new RSA TLS key and certificate and writes it to a file.
func newTLSKey(tlsKeyPath, tlsCertPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(certificateDuration)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}
	certOut, err := os.Create(tlsCertPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(tlsKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	if err = pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}); err != nil {
		return err
	}

	return nil
}
