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
			// Topic for user updates (profile, presence)
			UserUpdates Topic `yaml:"user_updates"`
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
		// The AppServices database stores information used by the AppService component.
		// It is only accessed by the AppService component.
		AppService DataSource `yaml:"appservice"`
		// The PublicRoomsAPI database stores information used to compute the public
		// room directory. It is only accessed by the PublicRoomsAPI server.
		PublicRoomsAPI DataSource `yaml:"public_rooms_api"`
		// The Naffka database is used internally by the naffka library, if used.
		Naffka DataSource `yaml:"naffka,omitempty"`
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
	Listen struct {
		MediaAPI         Address `yaml:"media_api"`
		ClientAPI        Address `yaml:"client_api"`
		FederationAPI    Address `yaml:"federation_api"`
		SyncAPI          Address `yaml:"sync_api"`
		RoomServer       Address `yaml:"room_server"`
		FederationSender Address `yaml:"federation_sender"`
		PublicRoomsAPI   Address `yaml:"public_rooms_api"`
	} `yaml:"listen"`

	// The config for tracing the dendrite servers.
	Tracing struct {
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

// Load a yaml config file for a server run as multiple processes.
// Checks the config to ensure that it is valid.
// The checks are different if the server is run as a monolithic process instead
// of being split into multiple components
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
	monolithic := false
	return loadConfig(basePath, configData, ioutil.ReadFile, monolithic)
}

// LoadMonolithic loads a yaml config file for a server run as a single monolith.
// Checks the config to ensure that it is valid.
// The checks are different if the server is run as a monolithic process instead
// of being split into multiple components
func LoadMonolithic(configPath string) (*Dendrite, error) {
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
	monolithic := true
	return loadConfig(basePath, configData, ioutil.ReadFile, monolithic)
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

	config.setDefaults()

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
	err = config.derive()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// derive generates data that is derived from various values provided in
// the config file.
func (config *Dendrite) derive() error {
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
	if err := loadAppservices(config); err != nil {
		return err
	}

	return nil
}

// setDefaults sets default config values if they are not explicitly set.
func (config *Dendrite) setDefaults() {
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
	checkNotEmpty(configErrs, "kafka.topics.user_updates", string(config.Kafka.Topics.UserUpdates))
}

// checkDatabase verifies the parameters database.* are valid.
func (config *Dendrite) checkDatabase(configErrs *configErrors) {
	checkNotEmpty(configErrs, "database.account", string(config.Database.Account))
	checkNotEmpty(configErrs, "database.device", string(config.Database.Device))
	checkNotEmpty(configErrs, "database.server_key", string(config.Database.ServerKey))
	checkNotEmpty(configErrs, "database.media_api", string(config.Database.MediaAPI))
	checkNotEmpty(configErrs, "database.sync_api", string(config.Database.SyncAPI))
	checkNotEmpty(configErrs, "database.room_server", string(config.Database.RoomServer))
}

// checkListen verifies the parameters listen.* are valid.
func (config *Dendrite) checkListen(configErrs *configErrors) {
	checkNotEmpty(configErrs, "listen.media_api", string(config.Listen.MediaAPI))
	checkNotEmpty(configErrs, "listen.client_api", string(config.Listen.ClientAPI))
	checkNotEmpty(configErrs, "listen.federation_api", string(config.Listen.FederationAPI))
	checkNotEmpty(configErrs, "listen.sync_api", string(config.Listen.SyncAPI))
	checkNotEmpty(configErrs, "listen.room_server", string(config.Listen.RoomServer))
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
	// In order to get the proper behavior, it is necessary to return an explicit nil
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

// RoomServerURL returns an HTTP URL for where the roomserver is listening.
func (config *Dendrite) RoomServerURL() string {
	// Hard code the roomserver to talk HTTP for now.
	// If we support HTTPS we need to think of a practical way to do certificate validation.
	// People setting up servers shouldn't need to get a certificate valid for the public
	// internet for an internal API.
	return "http://" + string(config.Listen.RoomServer)
}

// SetupTracing configures the opentracing using the supplied configuration.
func (config *Dendrite) SetupTracing(serviceName string) (closer io.Closer, err error) {
	return config.Tracing.Jaeger.InitGlobalTracer(
		serviceName,
		jaegerconfig.Logger(logrusLogger{logrus.StandardLogger()}),
		jaegerconfig.Metrics(jaegermetrics.NullFactory),
	)
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
