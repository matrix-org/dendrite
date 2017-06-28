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

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"
)

// Version is the current version of the config format.
// This will change whenever we make breaking changes to the config format.
const Version = "v0"

// Dendrite contains all the config used by a dendrite process.
// Relative paths are resolved relative to the current working directory
type Dendrite struct {
	// The version of the configuration file.
	// If the version in a file doesn't match the current dendrite config
	// version then we can give a clear error message telling the user
	// to update their config file to the current version.
	// The version of the file should only be different if there has
	// been a breaking change to the config file format.
	Version string `yaml:"version"`

	// The configuration required for a matrix server.
	Matrix struct {
		// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
		ServerName gomatrixserverlib.ServerName `yaml:"server_name"`
		// Path to the private key which will be used to sign requests and events.
		PrivateKeyPath Path `yaml:"private_key"`
		// The private key which will be used to sign requests and events.
		PrivateKey ed25519.PrivateKey `yaml:"-"`
		// An arbitrary string used to uniquely identify the PrivateKey. Must start with the
		// prefix "ed25519:".
		KeyID gomatrixserverlib.KeyID `yaml:"-"`
		// List of paths to X509 certificates used by the external federation listeners.
		// These are used to calculate the TLS fingerprints to publish for this server.
		// Other matrix servers talking to this server will expect the x509 certificate
		// to match one of these certificates.
		// The certificates should be in PEM format.
		FederationCertificatePaths []Path `yaml:"federation_certificates"`
		// A list of SHA256 TLS fingerprints for the X509 certificates used by the
		// federation listener for this server.
		TLSFingerPrints []gomatrixserverlib.TLSFingerprint `yaml:"-"`
		// How long a remote server can cache our server key for before requesting it again.
		// Increasing this number will reduce the number of requests made by remote servers
		// for our key, but increases the period a compromised key will be considered valid
		// by remote servers.
		// Defaults to 24 hours.
		KeyValidityPeriod time.Duration `yaml:"key_validity_period"`
	} `yaml:"matrix"`

	// The configuration specific to the media repostitory.
	Media struct {
		// The base path to where the media files will be stored. May be relative or absolute.
		BasePath Path `yaml:"base_path"`
		// The absolute base path to where media files will be stored.
		AbsBasePath Path `yaml:"-"`
		// The maximum file size in bytes that is allowed to be stored on this server.
		// Note: if max_file_size_bytes is set to 0, the size is unlimited.
		// Note: if max_file_size_bytes is not set, it will default to 10485760 (10MB)
		MaxFileSizeBytes *FileSizeBytes `yaml:"max_file_size_bytes,omitempty"`
		// Whether to dynamically generate thumbnails on-the-fly if the requested resolution is not already generated
		DynamicThumbnails bool `yaml:"dynamic_thumbnails"`
		// The maximum number of simultaneous thumbnail generators. default: 10
		MaxThumbnailGenerators int `yaml:"max_thumbnail_generators"`
		// A list of thumbnail sizes to be pre-generated for downloaded remote / uploaded content
		ThumbnailSizes []ThumbnailSize `yaml:"thumbnail_sizes"`
	} `yaml:"media"`

	// The configuration for talking to kafka.
	Kafka struct {
		// A list of kafka addresses to connect to.
		Addresses []string `yaml:"addresses"`
		// The names of the topics to use when reading and writing from kafka.
		Topics struct {
			// Topic for roomserver/api.InputRoomEvent events.
			InputRoomEvent Topic `yaml:"input_room_event"`
			// Topic for roomserver/api.OutputRoomEvent events.
			OutputRoomEvent Topic `yaml:"output_room_event"`
		}
	} `yaml:"kafka"`

	// Postgres Config
	Database struct {
		// The Account database stores the login details and account information
		// for local users. It is accessed by the ClientAPI.
		Account DataSource `yaml:"account"`
		// The Device database stores session information for the devices of logged
		// in local users. It is accessed by the ClientAPI, the MediaAPI and the SyncAPI.
		Device DataSource `yaml:"device"`
		// The MediaAPI database stores information about files uploaded and downloaded
		// by local users. It is only accessed by the MediaAPI.
		MediaAPI DataSource `yaml:"media_api"`
		// The ServerKey database caches the public keys of remote servers.
		// It may be accessed by the FederationAPI, the ClientAPI, and the MediaAPI.
		ServerKey DataSource `yaml:"server_key"`
		// The SyncAPI stores information used by the SyncAPI server.
		// It is only accessed by the SyncAPI server.
		SyncAPI DataSource `yaml:"sync_api"`
		// The RoomServer database stores information about matrix rooms.
		// It is only accessed by the RoomServer.
		RoomServer DataSource `yaml:"room_server"`
		// The FederationSender database stores information used by the FederationSender
		// It is only accessed by the FederationSender.
		FederationSender DataSource `yaml:"federation_sender"`
	} `yaml:"database"`

	// The internal addresses the components will listen on.
	// These should not be exposed externally as they expose metrics and debugging APIs.
	Listen struct {
		MediaAPI         Address `yaml:"media_api"`
		ClientAPI        Address `yaml:"client_api"`
		FederationAPI    Address `yaml:"federation_api"`
		SyncAPI          Address `yaml:"sync_api"`
		RoomServer       Address `yaml:"room_server"`
		FederationSender Address `yaml:"federation_sender"`
	} `yaml:"listen"`
}

// A Path on the filesystem.
type Path string

// A DataSource for opening a postgresql database using lib/pq.
type DataSource string

// A Topic in kafka.
type Topic string

// An Address to listen on.
type Address string

// FileSizeBytes is a file size in bytes
type FileSizeBytes int64

// ThumbnailSize contains a single thumbnail size configuration
type ThumbnailSize struct {
	// Maximum width of the thumbnail image
	Width int `yaml:"width"`
	// Maximum height of the thumbnail image
	Height int `yaml:"height"`
	// ResizeMethod is one of crop or scale.
	// crop scales to fill the requested dimensions and crops the excess.
	// scale scales to fit the requested dimensions and one dimension may be smaller than requested.
	ResizeMethod string `yaml:"method,omitempty"`
}

// Load a yaml config file
func Load(configPath string) (*Dendrite, error) {
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	basePath, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	// Pass the current working directory and ioutil.ReadFile so that they can
	// be mocked in the tests
	return loadConfig(basePath, configData, ioutil.ReadFile)
}

// An Error indicates a problem parsing the config.
type Error struct {
	// List of problems encountered parsing the config.
	Problems []string
}

func loadConfig(
	basePath string,
	configData []byte,
	readFile func(string) ([]byte, error),
) (*Dendrite, error) {
	var config Dendrite
	var err error
	if err = yaml.Unmarshal(configData, &config); err != nil {
		return nil, err
	}

	config.setDefaults()

	if err = config.check(); err != nil {
		return nil, err
	}

	privateKeyPath := absPath(basePath, config.Matrix.PrivateKeyPath)
	privateKeyData, err := readFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	if config.Matrix.KeyID, config.Matrix.PrivateKey, err = readKeyPEM(privateKeyPath, privateKeyData); err != nil {
		return nil, err
	}

	for _, certPath := range config.Matrix.FederationCertificatePaths {
		absCertPath := absPath(basePath, certPath)
		pemData, err := readFile(absCertPath)
		if err != nil {
			return nil, err
		}
		fingerprint := fingerprintPEM(pemData)
		if fingerprint == nil {
			return nil, fmt.Errorf("no certificate PEM data in %q", absCertPath)
		}
		config.Matrix.TLSFingerPrints = append(config.Matrix.TLSFingerPrints, *fingerprint)
	}

	config.Media.AbsBasePath = Path(absPath(basePath, config.Media.BasePath))

	return &config, nil
}

func (config *Dendrite) setDefaults() {
	if config.Matrix.KeyValidityPeriod == 0 {
		config.Matrix.KeyValidityPeriod = 24 * time.Hour
	}

	if config.Media.MaxThumbnailGenerators == 0 {
		config.Media.MaxThumbnailGenerators = 10
	}

	if config.Media.MaxFileSizeBytes == nil {
		defaultMaxFileSizeBytes := FileSizeBytes(10485760)
		config.Media.MaxFileSizeBytes = &defaultMaxFileSizeBytes
	}
}

func (e Error) Error() string {
	if len(e.Problems) == 1 {
		return e.Problems[0]
	}
	return fmt.Sprintf(
		"%s (and %d other problems)", e.Problems[0], len(e.Problems)-1,
	)
}

func (config *Dendrite) check() error {
	var problems []string

	if config.Version != Version {
		return Error{[]string{fmt.Sprintf(
			"unknown config version %q, expected %q", config.Version, Version,
		)}}
	}

	checkNotEmpty := func(key string, value string) {
		if value == "" {
			problems = append(problems, fmt.Sprintf("missing config key %q", key))
		}
	}

	checkNotZero := func(key string, value int64) {
		if value == 0 {
			problems = append(problems, fmt.Sprintf("missing config key %q", key))
		}
	}

	checkPositive := func(key string, value int64) {
		if value < 0 {
			problems = append(problems, fmt.Sprintf("invalid value for config key %q: %d", key, value))
		}
	}

	checkNotEmpty("matrix.server_name", string(config.Matrix.ServerName))
	checkNotEmpty("matrix.private_key", string(config.Matrix.PrivateKeyPath))
	checkNotZero("matrix.federation_certificates", int64(len(config.Matrix.FederationCertificatePaths)))

	checkNotEmpty("media.base_path", string(config.Media.BasePath))
	checkPositive("media.max_file_size_bytes", int64(*config.Media.MaxFileSizeBytes))
	checkPositive("media.max_thumbnail_generators", int64(config.Media.MaxThumbnailGenerators))
	for i, size := range config.Media.ThumbnailSizes {
		checkPositive(fmt.Sprintf("media.thumbnail_sizes[%d].width", i), int64(size.Width))
		checkPositive(fmt.Sprintf("media.thumbnail_sizes[%d].height", i), int64(size.Height))
	}

	checkNotZero("kafka.addresses", int64(len(config.Kafka.Addresses)))
	checkNotEmpty("kafka.topics.input_room_event", string(config.Kafka.Topics.InputRoomEvent))
	checkNotEmpty("kafka.topics.output_room_event", string(config.Kafka.Topics.OutputRoomEvent))
	checkNotEmpty("database.account", string(config.Database.Account))
	checkNotEmpty("database.device", string(config.Database.Device))
	checkNotEmpty("database.server_key", string(config.Database.ServerKey))
	checkNotEmpty("database.media_api", string(config.Database.MediaAPI))
	checkNotEmpty("database.sync_api", string(config.Database.SyncAPI))
	checkNotEmpty("database.room_server", string(config.Database.RoomServer))
	checkNotEmpty("listen.media_api", string(config.Listen.MediaAPI))
	checkNotEmpty("listen.client_api", string(config.Listen.ClientAPI))
	checkNotEmpty("listen.federation_api", string(config.Listen.FederationAPI))
	checkNotEmpty("listen.sync_api", string(config.Listen.SyncAPI))
	checkNotEmpty("listen.room_server", string(config.Listen.RoomServer))

	if problems != nil {
		return Error{problems}
	}

	return nil
}

func absPath(dir string, path Path) string {
	if filepath.IsAbs(string(path)) {
		// filepath.Join cleans the path so we should clean the absolute paths as well for consistency.
		return filepath.Clean(string(path))
	}
	return filepath.Join(dir, string(path))
}

func readKeyPEM(path string, data []byte) (gomatrixserverlib.KeyID, ed25519.PrivateKey, error) {
	for {
		var keyBlock *pem.Block
		keyBlock, data = pem.Decode(data)
		if data == nil {
			return "", nil, fmt.Errorf("no matrix private key PEM data in %q", path)
		}
		if keyBlock.Type == "MATRIX PRIVATE KEY" {
			keyID := keyBlock.Headers["Key-ID"]
			if keyID == "" {
				return "", nil, fmt.Errorf("missing key ID in PEM data in %q", path)
			}
			if !strings.HasPrefix(keyID, "ed25519:") {
				return "", nil, fmt.Errorf("key ID %q doesn't start with \"ed25519:\" in %q", keyID, path)
			}
			_, privKey, err := ed25519.GenerateKey(bytes.NewReader(keyBlock.Bytes))
			if err != nil {
				return "", nil, err
			}
			return gomatrixserverlib.KeyID(keyID), privKey, nil
		}
	}
}

func fingerprintPEM(data []byte) *gomatrixserverlib.TLSFingerprint {
	for {
		var certDERBlock *pem.Block
		certDERBlock, data = pem.Decode(data)
		if data == nil {
			return nil
		}
		if certDERBlock.Type == "CERTIFICATE" {
			digest := sha256.Sum256(certDERBlock.Bytes)
			return &gomatrixserverlib.TLSFingerprint{digest[:]}
		}
	}
}

// RoomServerURL returns an HTTP URL for where the roomserver is listening.
func (config *Dendrite) RoomServerURL() string {
	// Hard code the roomserver to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.RoomServer)
}
