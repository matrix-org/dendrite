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
	"io"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	yaml "gopkg.in/yaml.v2"

	jaegerconfig "github.com/uber/jaeger-client-go/config"
	jaegermetrics "github.com/uber/jaeger-lib/metrics"
)

// Version is the current version of the config format.
// This will change whenever we make breaking changes to the config format.
const Version = 0

// Dendrite contains all the config used by a dendrite process.
// Relative paths are resolved relative to the current working directory
type Dendrite struct {
	// The version of the configuration file.
	// If the version in a file doesn't match the current dendrite config
	// version then we can give a clear error message telling the user
	// to update their config file to the current version.
	// The version of the file should only be different if there has
	// been a breaking change to the config file format.
	Version int `yaml:"version"`

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
		// List of domains that the server will trust as identity servers to
		// verify third-party identifiers.
		// Defaults to an empty array.
		TrustedIDServers []string `yaml:"trusted_third_party_id_servers"`
		// If set, allows registration by anyone who also has the shared
		// secret, even if registration is otherwise disabled.
		RegistrationSharedSecret string `yaml:"registration_shared_secret"`
		// This Home Server's ReCAPTCHA public key.
		RecaptchaPublicKey string `yaml:"recaptcha_public_key"`
		// This Home Server's ReCAPTCHA private key.
		RecaptchaPrivateKey string `yaml:"recaptcha_private_key"`
		// Boolean stating whether catpcha registration is enabled
		// and required
		RecaptchaEnabled bool `yaml:"enable_registration_captcha"`
		// Secret used to bypass the captcha registration entirely
		RecaptchaBypassSecret string `yaml:"captcha_bypass_secret"`
		// HTTP API endpoint used to verify whether the captcha response
		// was successful
		RecaptchaSiteVerifyAPI string `yaml:"recaptcha_siteverify_api"`
		// If set disables new users from registering (except via shared
		// secrets)
		RegistrationDisabled bool `yaml:"registration_disabled"`
		// Perspective keyservers, to use as a backup when direct key fetch
		// requests don't succeed
		KeyPerspectives KeyPerspectives `yaml:"key_perspectives"`
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

	// The configuration to use for Prometheus metrics
	Metrics struct {
		// Whether or not the metrics are enabled
		Enabled bool `yaml:"enabled"`
		// Use BasicAuth for Authorization
		BasicAuth struct {
			// Authorization via Static Username & Password
			// Hardcoded Username and Password
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"basic_auth"`
	} `yaml:"metrics"`

	// The configuration for talking to kafka.
	Kafka struct {
		// A list of kafka addresses to connect to.
		Addresses []string `yaml:"addresses"`
		// Whether to use naffka instead of kafka.
		// Naffka can only be used when running dendrite as a single monolithic server.
		// Kafka can be used both with a monolithic server and when running the
		// components as separate servers.
		UseNaffka bool `yaml:"use_naffka,omitempty"`
		// The names of the topics to use when reading and writing from kafka.
		Topics struct {
			// Topic for roomserver/api.OutputRoomEvent events.
			OutputRoomEvent Topic `yaml:"output_room_event"`
			// Topic for sending account data from client API to sync API
			OutputClientData Topic `yaml:"output_client_data"`
			// Topic for eduserver/api.OutputTypingEvent events.
			OutputTypingEvent Topic `yaml:"output_typing_event"`
			// Topic for eduserver/api.OutputSendToDeviceEvent events.
			OutputSendToDeviceEvent Topic `yaml:"output_send_to_device_event"`
		}
	} `yaml:"kafka"`

	// Postgres Config
	Database struct {
		// The Account database stores the login details and account information
		// for local users. It is accessed by the UserAPI.
		Account DataSource `yaml:"account"`
		// The CurrentState database stores the current state of all rooms.
		// It is accessed by the CurrentStateServer.
		CurrentState DataSource `yaml:"current_state"`
		// The Device database stores session information for the devices of logged
		// in local users. It is accessed by the UserAPI.
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
		// The AppServices database stores information used by the AppService component.
		// It is only accessed by the AppService component.
		AppService DataSource `yaml:"appservice"`
		// The Naffka database is used internally by the naffka library, if used.
		Naffka DataSource `yaml:"naffka,omitempty"`
		// Maximum open connections to the DB (0 = use default, negative means unlimited)
		MaxOpenConns int `yaml:"max_open_conns"`
		// Maximum idle connections to the DB (0 = use default, negative means unlimited)
		MaxIdleConns int `yaml:"max_idle_conns"`
		// maximum amount of time (in seconds) a connection may be reused (<= 0 means unlimited)
		ConnMaxLifetimeSec int `yaml:"conn_max_lifetime"`
	} `yaml:"database"`

	// TURN Server Config
	TURN struct {
		// TODO Guest Support
		// Whether or not guests can request TURN credentials
		//AllowGuests bool `yaml:"turn_allow_guests"`
		// How long the authorization should last
		UserLifetime string `yaml:"turn_user_lifetime"`
		// The list of TURN URIs to pass to clients
		URIs []string `yaml:"turn_uris"`

		// Authorization via Shared Secret
		// The shared secret from coturn
		SharedSecret string `yaml:"turn_shared_secret"`

		// Authorization via Static Username & Password
		// Hardcoded Username and Password
		Username string `yaml:"turn_username"`
		Password string `yaml:"turn_password"`
	} `yaml:"turn"`

	// The internal addresses the components will listen on.
	// These should not be exposed externally as they expose metrics and debugging APIs.
	// Falls back to addresses listed in Listen if not specified
	Bind struct {
		MediaAPI         Address `yaml:"media_api"`
		ClientAPI        Address `yaml:"client_api"`
		CurrentState     Address `yaml:"current_state_server"`
		FederationAPI    Address `yaml:"federation_api"`
		ServerKeyAPI     Address `yaml:"server_key_api"`
		AppServiceAPI    Address `yaml:"appservice_api"`
		SyncAPI          Address `yaml:"sync_api"`
		UserAPI          Address `yaml:"user_api"`
		RoomServer       Address `yaml:"room_server"`
		FederationSender Address `yaml:"federation_sender"`
		EDUServer        Address `yaml:"edu_server"`
		KeyServer        Address `yaml:"key_server"`
	} `yaml:"bind"`

	// The addresses for talking to other microservices.
	Listen struct {
		MediaAPI         Address `yaml:"media_api"`
		ClientAPI        Address `yaml:"client_api"`
		CurrentState     Address `yaml:"current_state_server"`
		FederationAPI    Address `yaml:"federation_api"`
		ServerKeyAPI     Address `yaml:"server_key_api"`
		AppServiceAPI    Address `yaml:"appservice_api"`
		SyncAPI          Address `yaml:"sync_api"`
		UserAPI          Address `yaml:"user_api"`
		RoomServer       Address `yaml:"room_server"`
		FederationSender Address `yaml:"federation_sender"`
		EDUServer        Address `yaml:"edu_server"`
		KeyServer        Address `yaml:"key_server"`
	} `yaml:"listen"`

	// The config for tracing the dendrite servers.
	Tracing struct {
		// Set to true to enable tracer hooks. If false, no tracing is set up.
		Enabled bool `yaml:"enabled"`
		// The config for the jaeger opentracing reporter.
		Jaeger jaegerconfig.Configuration `yaml:"jaeger"`
	} `yaml:"tracing"`

	// Application Services
	// https://matrix.org/docs/spec/application_service/unstable.html
	ApplicationServices struct {
		// Configuration files for various application services
		ConfigFiles []string `yaml:"config_files"`
	} `yaml:"application_services"`

	// The config for logging informations. Each hook will be added to logrus.
	Logging []LogrusHook `yaml:"logging"`

	// The config for setting a proxy to use for server->server requests
	Proxy *struct {
		// The protocol for the proxy (http / https / socks5)
		Protocol string `yaml:"protocol"`
		// The host where the proxy is listening
		Host string `yaml:"host"`
		// The port on which the proxy is listening
		Port uint16 `yaml:"port"`
	} `yaml:"proxy"`

	// Any information derived from the configuration options for later use.
	Derived struct {
		Registration struct {
			// Flows is a slice of flows, which represent one possible way that the client can authenticate a request.
			// http://matrix.org/docs/spec/HEAD/client_server/r0.3.0.html#user-interactive-authentication-api
			// As long as the generated flows only rely on config file options,
			// we can generate them on startup and store them until needed
			Flows []authtypes.Flow `json:"flows"`

			// Params that need to be returned to the client during
			// registration in order to complete registration stages.
			Params map[string]interface{} `json:"params"`
		}

		// Application services parsed from their config files
		// The paths of which were given above in the main config file
		ApplicationServices []ApplicationService

		// Meta-regexes compiled from all exclusive application service
		// Regexes.
		//
		// When a user registers, we check that their username does not match any
		// exclusive application service namespaces
		ExclusiveApplicationServicesUsernameRegexp *regexp.Regexp
		// When a user creates a room alias, we check that it isn't already
		// reserved by an application service
		ExclusiveApplicationServicesAliasRegexp *regexp.Regexp
		// Note: An Exclusive Regex for room ID isn't necessary as we aren't blocking
		// servers from creating RoomIDs in exclusive application service namespaces
	} `yaml:"-"`
}

// KeyPerspectives are used to configure perspective key servers for
// retrieving server keys.
type KeyPerspectives []struct {
	// The server name of the perspective key server
	ServerName gomatrixserverlib.ServerName `yaml:"server_name"`
	// Server keys for the perspective user, used to verify the
	// keys have been signed by the perspective server
	Keys []struct {
		// The key ID, e.g. ed25519:auto
		KeyID gomatrixserverlib.KeyID `yaml:"key_id"`
		// The public key in base64 unpadded format
		PublicKey string `yaml:"public_key"`
	} `yaml:"keys"`
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

// LogrusHook represents a single logrus hook. At this point, only parsing and
// verification of the proper values for type and level are done.
// Validity/integrity checks on the parameters are done when configuring logrus.
type LogrusHook struct {
	// The type of hook, currently only "file" is supported.
	Type string `yaml:"type"`

	// The level of the logs to produce. Will output only this level and above.
	Level string `yaml:"level"`

	// The parameters for this hook.
	Params map[string]interface{} `yaml:"params"`
}

// configErrors stores problems encountered when parsing a config file.
// It implements the error interface.
type configErrors []string

// Load a yaml config file for a server run as multiple processes or as a monolith.
// Checks the config to ensure that it is valid.
func Load(configPath string, monolith bool) (*Dendrite, error) {
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
	return loadConfig(basePath, configData, ioutil.ReadFile, monolith)
}

func loadConfig(
	basePath string,
	configData []byte,
	readFile func(string) ([]byte, error),
	monolithic bool,
) (*Dendrite, error) {
	var config Dendrite
	var err error
	if err = yaml.Unmarshal(configData, &config); err != nil {
		return nil, err
	}

	config.SetDefaults()

	if err = config.check(monolithic); err != nil {
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
		var pemData []byte
		pemData, err = readFile(absCertPath)
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

	// Generate data from config options
	err = config.Derive()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// Derive generates data that is derived from various values provided in
// the config file.
func (config *Dendrite) Derive() error {
	// Determine registrations flows based off config values

	config.Derived.Registration.Params = make(map[string]interface{})

	// TODO: Add email auth type
	// TODO: Add MSISDN auth type

	if config.Matrix.RecaptchaEnabled {
		config.Derived.Registration.Params[authtypes.LoginTypeRecaptcha] = map[string]string{"public_key": config.Matrix.RecaptchaPublicKey}
		config.Derived.Registration.Flows = append(config.Derived.Registration.Flows,
			authtypes.Flow{Stages: []authtypes.LoginType{authtypes.LoginTypeRecaptcha}})
	} else {
		config.Derived.Registration.Flows = append(config.Derived.Registration.Flows,
			authtypes.Flow{Stages: []authtypes.LoginType{authtypes.LoginTypeDummy}})
	}

	// Load application service configuration files
	if err := loadAppServices(config); err != nil {
		return err
	}

	return nil
}

// SetDefaults sets default config values if they are not explicitly set.
func (config *Dendrite) SetDefaults() {
	if config.Matrix.KeyValidityPeriod == 0 {
		config.Matrix.KeyValidityPeriod = 24 * time.Hour
	}

	if config.Matrix.TrustedIDServers == nil {
		config.Matrix.TrustedIDServers = []string{}
	}

	if config.Media.MaxThumbnailGenerators == 0 {
		config.Media.MaxThumbnailGenerators = 10
	}

	if config.Media.MaxFileSizeBytes == nil {
		defaultMaxFileSizeBytes := FileSizeBytes(10485760)
		config.Media.MaxFileSizeBytes = &defaultMaxFileSizeBytes
	}

	if config.Database.MaxIdleConns == 0 {
		config.Database.MaxIdleConns = 2
	}

	if config.Database.MaxOpenConns == 0 {
		config.Database.MaxOpenConns = 100
	}

}

// Error returns a string detailing how many errors were contained within a
// configErrors type.
func (errs configErrors) Error() string {
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Sprintf(
		"%s (and %d other problems)", errs[0], len(errs)-1,
	)
}

// Add appends an error to the list of errors in this configErrors.
// It is a wrapper to the builtin append and hides pointers from
// the client code.
// This method is safe to use with an uninitialized configErrors because
// if it is nil, it will be properly allocated.
func (errs *configErrors) Add(str string) {
	*errs = append(*errs, str)
}

// checkNotEmpty verifies the given value is not empty in the configuration.
// If it is, adds an error to the list.
func checkNotEmpty(configErrs *configErrors, key, value string) {
	if value == "" {
		configErrs.Add(fmt.Sprintf("missing config key %q", key))
	}
}

// checkNotZero verifies the given value is not zero in the configuration.
// If it is, adds an error to the list.
func checkNotZero(configErrs *configErrors, key string, value int64) {
	if value == 0 {
		configErrs.Add(fmt.Sprintf("missing config key %q", key))
	}
}

// checkPositive verifies the given value is positive (zero included)
// in the configuration. If it is not, adds an error to the list.
func checkPositive(configErrs *configErrors, key string, value int64) {
	if value < 0 {
		configErrs.Add(fmt.Sprintf("invalid value for config key %q: %d", key, value))
	}
}

// checkTurn verifies the parameters turn.* are valid.
func (config *Dendrite) checkTurn(configErrs *configErrors) {
	value := config.TURN.UserLifetime
	if value != "" {
		if _, err := time.ParseDuration(value); err != nil {
			configErrs.Add(fmt.Sprintf("invalid duration for config key %q: %s", "turn.turn_user_lifetime", value))
		}
	}
}

// checkMatrix verifies the parameters matrix.* are valid.
func (config *Dendrite) checkMatrix(configErrs *configErrors) {
	checkNotEmpty(configErrs, "matrix.server_name", string(config.Matrix.ServerName))
	checkNotEmpty(configErrs, "matrix.private_key", string(config.Matrix.PrivateKeyPath))
	checkNotZero(configErrs, "matrix.federation_certificates", int64(len(config.Matrix.FederationCertificatePaths)))
	if config.Matrix.RecaptchaEnabled {
		checkNotEmpty(configErrs, "matrix.recaptcha_public_key", string(config.Matrix.RecaptchaPublicKey))
		checkNotEmpty(configErrs, "matrix.recaptcha_private_key", string(config.Matrix.RecaptchaPrivateKey))
		checkNotEmpty(configErrs, "matrix.recaptcha_siteverify_api", string(config.Matrix.RecaptchaSiteVerifyAPI))
	}
}

// checkMedia verifies the parameters media.* are valid.
func (config *Dendrite) checkMedia(configErrs *configErrors) {
	checkNotEmpty(configErrs, "media.base_path", string(config.Media.BasePath))
	checkPositive(configErrs, "media.max_file_size_bytes", int64(*config.Media.MaxFileSizeBytes))
	checkPositive(configErrs, "media.max_thumbnail_generators", int64(config.Media.MaxThumbnailGenerators))

	for i, size := range config.Media.ThumbnailSizes {
		checkPositive(configErrs, fmt.Sprintf("media.thumbnail_sizes[%d].width", i), int64(size.Width))
		checkPositive(configErrs, fmt.Sprintf("media.thumbnail_sizes[%d].height", i), int64(size.Height))
	}
}

// checkKafka verifies the parameters kafka.* and the related
// database.naffka are valid.
func (config *Dendrite) checkKafka(configErrs *configErrors, monolithic bool) {

	if config.Kafka.UseNaffka {
		if !monolithic {
			configErrs.Add(fmt.Sprintf("naffka can only be used in a monolithic server"))
		}

		checkNotEmpty(configErrs, "database.naffka", string(config.Database.Naffka))
	} else {
		// If we aren't using naffka then we need to have at least one kafka
		// server to talk to.
		checkNotZero(configErrs, "kafka.addresses", int64(len(config.Kafka.Addresses)))
	}
	checkNotEmpty(configErrs, "kafka.topics.output_room_event", string(config.Kafka.Topics.OutputRoomEvent))
	checkNotEmpty(configErrs, "kafka.topics.output_client_data", string(config.Kafka.Topics.OutputClientData))
	checkNotEmpty(configErrs, "kafka.topics.output_typing_event", string(config.Kafka.Topics.OutputTypingEvent))
}

// checkDatabase verifies the parameters database.* are valid.
func (config *Dendrite) checkDatabase(configErrs *configErrors) {
	checkNotEmpty(configErrs, "database.account", string(config.Database.Account))
	checkNotEmpty(configErrs, "database.device", string(config.Database.Device))
	checkNotEmpty(configErrs, "database.server_key", string(config.Database.ServerKey))
	checkNotEmpty(configErrs, "database.media_api", string(config.Database.MediaAPI))
	checkNotEmpty(configErrs, "database.sync_api", string(config.Database.SyncAPI))
	checkNotEmpty(configErrs, "database.room_server", string(config.Database.RoomServer))
	checkNotEmpty(configErrs, "database.current_state", string(config.Database.CurrentState))
}

// checkListen verifies the parameters listen.* are valid.
func (config *Dendrite) checkListen(configErrs *configErrors) {
	checkNotEmpty(configErrs, "listen.media_api", string(config.Listen.MediaAPI))
	checkNotEmpty(configErrs, "listen.client_api", string(config.Listen.ClientAPI))
	checkNotEmpty(configErrs, "listen.federation_api", string(config.Listen.FederationAPI))
	checkNotEmpty(configErrs, "listen.sync_api", string(config.Listen.SyncAPI))
	checkNotEmpty(configErrs, "listen.room_server", string(config.Listen.RoomServer))
	checkNotEmpty(configErrs, "listen.edu_server", string(config.Listen.EDUServer))
	checkNotEmpty(configErrs, "listen.server_key_api", string(config.Listen.EDUServer))
	checkNotEmpty(configErrs, "listen.user_api", string(config.Listen.UserAPI))
	checkNotEmpty(configErrs, "listen.current_state_server", string(config.Listen.CurrentState))
}

// checkLogging verifies the parameters logging.* are valid.
func (config *Dendrite) checkLogging(configErrs *configErrors) {
	for _, logrusHook := range config.Logging {
		checkNotEmpty(configErrs, "logging.type", string(logrusHook.Type))
		checkNotEmpty(configErrs, "logging.level", string(logrusHook.Level))
	}
}

// check returns an error type containing all errors found within the config
// file.
func (config *Dendrite) check(monolithic bool) error {
	var configErrs configErrors

	if config.Version != Version {
		configErrs.Add(fmt.Sprintf(
			"unknown config version %q, expected %q", config.Version, Version,
		))
		return configErrs
	}

	config.checkMatrix(&configErrs)
	config.checkMedia(&configErrs)
	config.checkTurn(&configErrs)
	config.checkKafka(&configErrs, monolithic)
	config.checkDatabase(&configErrs)
	config.checkLogging(&configErrs)

	if !monolithic {
		config.checkListen(&configErrs)
	}

	// Due to how Golang manages its interface types, this condition is not redundant.
	// In order to get the proper behaviour, it is necessary to return an explicit nil
	// and not a nil configErrors.
	// This is because the following equalities hold:
	// error(nil) == nil
	// error(configErrors(nil)) != nil
	if configErrs != nil {
		return configErrs
	}
	return nil
}

// absPath returns the absolute path for a given relative or absolute path.
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
		if keyBlock == nil {
			return "", nil, fmt.Errorf("keyBlock is nil %q", path)
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
			return &gomatrixserverlib.TLSFingerprint{SHA256: digest[:]}
		}
	}
}

// AppServiceURL returns a HTTP URL for where the appservice component is listening.
func (config *Dendrite) AppServiceURL() string {
	// Hard code the appservice server to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.AppServiceAPI)
}

// RoomServerURL returns an HTTP URL for where the roomserver is listening.
func (config *Dendrite) RoomServerURL() string {
	// Hard code the roomserver to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.RoomServer)
}

// UserAPIURL returns an HTTP URL for where the userapi is listening.
func (config *Dendrite) UserAPIURL() string {
	// Hard code the userapi to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.UserAPI)
}

// CurrentStateAPIURL returns an HTTP URL for where the currentstateserver is listening.
func (config *Dendrite) CurrentStateAPIURL() string {
	// Hard code the currentstateserver to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.CurrentState)
}

// EDUServerURL returns an HTTP URL for where the EDU server is listening.
func (config *Dendrite) EDUServerURL() string {
	// Hard code the EDU server to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.EDUServer)
}

// FederationSenderURL returns an HTTP URL for where the federation sender is listening.
func (config *Dendrite) FederationSenderURL() string {
	// Hard code the federation sender server to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.FederationSender)
}

// ServerKeyAPIURL returns an HTTP URL for where the federation sender is listening.
func (config *Dendrite) ServerKeyAPIURL() string {
	// Hard code the server key API server to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.ServerKeyAPI)
}

// SetupTracing configures the opentracing using the supplied configuration.
func (config *Dendrite) SetupTracing(serviceName string) (closer io.Closer, err error) {
	if !config.Tracing.Enabled {
		return ioutil.NopCloser(bytes.NewReader([]byte{})), nil
	}
	return config.Tracing.Jaeger.InitGlobalTracer(
		serviceName,
		jaegerconfig.Logger(logrusLogger{logrus.StandardLogger()}),
		jaegerconfig.Metrics(jaegermetrics.NullFactory),
	)
}

// MaxIdleConns returns maximum idle connections to the DB
func (config Dendrite) MaxIdleConns() int {
	return config.Database.MaxIdleConns
}

// MaxOpenConns returns maximum open connections to the DB
func (config Dendrite) MaxOpenConns() int {
	return config.Database.MaxOpenConns
}

// ConnMaxLifetime returns maximum amount of time a connection may be reused
func (config Dendrite) ConnMaxLifetime() time.Duration {
	return time.Duration(config.Database.ConnMaxLifetimeSec) * time.Second
}

// DbProperties functions return properties used by database/sql/DB
type DbProperties interface {
	MaxIdleConns() int
	MaxOpenConns() int
	ConnMaxLifetime() time.Duration
}

// DbProperties returns cfg as a DbProperties interface
func (config Dendrite) DbProperties() DbProperties {
	return config
}

// logrusLogger is a small wrapper that implements jaeger.Logger using logrus.
type logrusLogger struct {
	l *logrus.Logger
}

func (l logrusLogger) Error(msg string) {
	l.l.Error(msg)
}

func (l logrusLogger) Infof(msg string, args ...interface{}) {
	l.l.Infof(msg, args...)
}
