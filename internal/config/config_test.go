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
	"fmt"
	"testing"
)

func TestLoadConfigRelative(t *testing.T) {
	_, err := loadConfig("/my/config/dir", []byte(testConfig),
		mockReadFile{
			"/my/config/dir/matrix_key.pem": testKey,
			"/my/config/dir/tls_cert.pem":   testCert,
		}.readFile,
		false,
	)
	if err != nil {
		t.Error("failed to load config:", err)
	}
}

const testConfig = `
{
Version: 1
Global:
{
	# The domain name of this homeserver.
	ServerName: localhost

	# The path to the signing private key file, used to sign requests and events.
	PrivateKeyPath: matrix.pem

	# A unique identifier for this private key. Must start with the prefix "ed25519:".
	KeyID: ed25519:auto

	# How long a remote server can cache our server signing key before requesting it
	# again. Increasing this number will reduce the number of requests made by other
	# servers for our key but increases the period that a compromised key will be
	# considered valid by other homeservers.
	KeyValidityPeriod: 604800000000000

	# Lists of domains that the server will trust as identity servers to verify third
	# party identifiers such as phone numbers and email addresses.
	TrustedIDServers: []

	# Configuration for Kaffka/Naffka.
	Kafka:
	{
	# List of Kafka addresses to connect to.
	Addresses: []

	# The prefix to use for Kafka topic names for this homeserver. Change this only if
	# you are running more than one Dendrite homeserver on the same Kafka deployment.
	TopicPrefix: Dendrite

	# Whether to use Naffka instead of Kafka. Only available in monolith mode.
	UseNaffka: true

	# Naffka database options. Not required when using Kafka.
	NaffkaDatabase:
	{
		# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
		ConnectionString: file:naffka.db

		# Maximum number of connections to open to the database (0 = use default,
		# negative = unlimited).
		MaxOpenConnections: 100

		# Maximum number of idle connections permitted to the database (0 = use default,
		# negative = unlimited).
		MaxIdleConnections: 2

		# Maximum amount of time, in seconds, that a database connection may be reused
		# (negative = unlimited).
		ConnMaxLifetimeSeconds: -1
	}
	}

	# Configuration for Prometheus metric collection.
	Metrics:
	{
	# Whether or not Prometheus metrics are enabled.
	Enabled: false

	# HTTP basic authentication to protect access to monitoring.
	BasicAuth:
	{
		Username: metrics
		Password: metrics
	}
	}
}
AppServiceAPI:
{
	# Listen address for this component.
	Listen: localhost:7777

	# Bind address for this component.
	Bind: localhost:7777

	# Database options for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:appservice.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}

	# List of paths to appservice configuration files.
	ConfigFiles: []
}
ClientAPI:
{
	# The listen address for this component.
	Listen: localhost:7771

	# The bind address for this component.
	Bind: localhost:7771

	# Prevent new users from registering, except when using the shared secret from the
	# RegistrationSharedSecret option below.
	RegistrationDisabled: false

	# If set, allows registration by anyone who knows the shared secret, even if
	# registration is otherwise disabled.
	RegistrationSharedSecret: ""

	# Whether to require ReCAPTCHA for registration.
	RecaptchaEnabled: false

	# This server's ReCAPTCHA public key.
	RecaptchaPublicKey: ""

	# This server's ReCAPTCHA private key.
	RecaptchaPrivateKey: ""

	# Secret used to bypass ReCAPTCHA entirely.
	RecaptchaBypassSecret: ""

	# The URL to use for verifying if the ReCAPTCHA response was successful.
	RecaptchaSiteVerifyAPI: ""

	TURN:
	{
	# How long the TURN authorisation should last.
	UserLifetime: ""

	# The list of TURN URIs to pass to clients.
	URIs: []

	# Authorisation shared secret from coturn.
	SharedSecret: ""

	# Authorisation static username.
	Username: ""

	# Authorisation static password.
	Password: ""
	}
}
CurrentStateServer:
{
	# Listen address for this component.
	Listen: localhost:7782

	# Bind address for this component.
	Bind: localhost:7782

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:currentstate.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}
}
EDUServer:
{
	# Listen address for this component.
	Listen: localhost:7778

	# Bind address for this component.
	Bind: localhost:7778
}
FederationAPI:
{
	# Listen address for this component.
	Listen: localhost:7772

	# Bind address for this component.
	Bind: localhost:7772

	# List of paths to X.509 certificates to be used by the external federation listeners.
	# These certificates will be used to calculate the TLS fingerprints and other servers
	# will expect the certificate to match these fingerprints. Certificates must be in PEM
	# format.
	FederationCertificates: []

}
FederationSender:
{
	# Listen address for this component.
	Listen: localhost:7775

	# Bind address for this component.
	Bind: localhost:7775

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:federationsender.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}

	# How many times we will try to resend a failed transaction to a specific server. The
	# backoff is 2**x seconds, so 1 = 2 seconds, 2 = 4 seconds, 3 = 8 seconds etc.
	SendMaxRetries: 16

	# Disable the validation of TLS certificates of remote federated homeservers. Do not
	# enable this option in production as it presents a security risk!
	DisableTLSValidation: false

	# Use the following proxy server for outbound federation traffic.
	ProxyOutbound:
	{
	Enabled: false
	Protocol: http
	Host: localhost
	Port: 8080
	}
}
KeyServer:
{
	# Listen address for this component.
	Listen: localhost:7779

	# Bind address for this component.
	Bind: localhost:7779

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:keyserver.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}
}
MediaAPI:
{
	# Listen address for this component.
	Listen: localhost:7774

	# Bind address for this component.
	Bind: localhost:7774

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:mediaapi.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}

	# Storage path for uploaded media. May be relative or absolute.
	BasePath: ./media_store

	# The maximum allowed file size (in bytes) for media uploads to this homeserver
	# (0 = unlimited).
	MaxFileSizeBytes: 10485760

	# Whether to dynamically generate thumbnails if needed.
	DynamicThumbnails: false

	# The maximum number of simultaneous thumbnail generators to run.
	MaxThumbnailGenerators: 10

	# A list of thumbnail sizes to be generated for media content.
	ThumbnailSizes: []
}
RoomServer:
{
	# Listen address for this component.
	Listen: localhost:7770

	# Bind address for this component.
	Bind: localhost:7770

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:roomserver.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}
}
ServerKeyAPI:
{
	# Listen address for this component.
	Listen: localhost:7780

	# Bind address for this component.
	Bind: localhost:7780

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:serverkeyapi.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}

	# Perspective keyservers to use as a backup when direct key fetches fail.
	PerspectiveServers:
	[
	{
		ServerName: matrix.org
		TrustKeys:
		[
		{
			KeyID: ed25519:auto
			PublicKey: Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw
		}
		{
			KeyID: ed25519:a_RXGa
			PublicKey: l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ
		}
		]
	}
	]
}
SyncAPI:
{
	# Listen address for this component.
	Listen: localhost:7773

	# Bind address for this component.
	Bind: localhost:7773

	# Database configuration for this component.
	Database:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:syncapi.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}
}
UserAPI:
{
	# Listen address for this component.
	Listen: localhost:7781

	# Bind address for this component.
	Bind: localhost:7781

	# Database configuration for the account database.
	AccountDatabase:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:userapi_accounts.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}

	# Database configuration for the device database.
	DeviceDatabase:
	{
	# Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc.
	ConnectionString: file:userapi_devices.db

	# Maximum number of connections to open to the database (0 = use default,
	# negative = unlimited).
	MaxOpenConnections: 100

	# Maximum number of idle connections permitted to the database (0 = use default,
	# negative = unlimited).
	MaxIdleConnections: 2

	# Maximum amount of time, in seconds, that a database connection may be reused
	# (negative = unlimited).
	ConnMaxLifetimeSeconds: -1
	}
}
Tracing:
{
	Enabled: false
	Jaeger:
	{
	ServiceName: ""
	Disabled: false
	RPCMetrics: false
	Tags: []
	Sampler: null
	Reporter: null
	Headers: null
	BaggageRestrictions: null
	Throttler: null
	}
}
Logging:
[
	{
	Type: file
	Level: info
	Params:
	{
		path: /var/log/dendrite
	}
	}
]
}  
`

type mockReadFile map[string]string

func (m mockReadFile) readFile(path string) ([]byte, error) {
	data, ok := m[path]
	if !ok {
		return nil, fmt.Errorf("no such file %q", path)
	}
	return []byte(data), nil
}

func TestReadKey(t *testing.T) {
	keyID, _, err := readKeyPEM("path/to/key", []byte(testKey))
	if err != nil {
		t.Error("failed to load private key:", err)
	}
	wantKeyID := testKeyID
	if wantKeyID != string(keyID) {
		t.Errorf("wanted key ID to be %q, got %q", wantKeyID, keyID)
	}
}

const testKeyID = "ed25519:c8NsuQ"

const testKey = `
-----BEGIN MATRIX PRIVATE KEY-----
Key-ID: ` + testKeyID + `
7KRZiZ2sTyRR8uqqUjRwczuwRXXkUMYIUHq4Mc3t4bE=
-----END MATRIX PRIVATE KEY-----
`

func TestFingerprintPEM(t *testing.T) {
	got := fingerprintPEM([]byte(testCert))
	if got == nil {
		t.Error("failed to calculate fingerprint")
	}
	if string(got.SHA256) != testCertFingerprint {
		t.Errorf("bad fingerprint: wanted %q got %q", got, testCertFingerprint)
	}

}

const testCertFingerprint = "56.\\SPQxE\xd4\x95\xfb\xf6\xd5\x04\x91\xcb/\x07\xb1^\x88\x08\xe3\xc1p\xdfY\x04\x19w\xcb"

const testCert = `
-----BEGIN CERTIFICATE-----
MIIE0zCCArugAwIBAgIJAPype3u24LJeMA0GCSqGSIb3DQEBCwUAMAAwHhcNMTcw
NjEzMTQyODU4WhcNMTgwNjEzMTQyODU4WjAAMIICIjANBgkqhkiG9w0BAQEFAAOC
Ag8AMIICCgKCAgEA3vNSr7lCh/alxPFqairp/PYohwdsqPvOD7zf7dJCNhy0gbdC
9/APwIbPAPL9nU+o9ud1ACNCKBCQin/9LnI5vd5pa/Ne+mmRADDLB/BBBoywSJWG
NSfKJ9n3XY1bjgtqi53uUh+RDdQ7sXudDqCUxiiJZmS7oqK/mp88XXAgCbuXUY29
GmzbbDz37vntuSxDgUOnJ8uPSvRp5YPKogA3JwW1SyrlLt4Z30CQ6nH3Y2Q5SVfJ
NIQyMrnwyjA9bCdXezv1cLXoTYn7U9BRyzXTZeXs3y3ldnRfISXN35CU04Az1F8j
lfj7nXMEqI/qAj/qhxZ8nVBB+rpNOZy9RJko3O+G5Qa/EvzkQYV1rW4TM2Yme88A
QyJspoV/0bXk6gG987PonK2Uk5djxSULhnGVIqswydyH0Nzb+slRp2bSoWbaNlee
+6TIeiyTQYc055pCHOp22gtLrC5LQGchksi02St2ZzRHdnlfqCJ8S9sS7x3trzds
cYueg1sGI+O8szpQ3eUM7OhJOBrx6OlR7+QYnQg1wr/V+JAz1qcyTC1URcwfeqtg
QjxFdBD9LfCtfK+AO51H9ugtsPJqOh33PmvfvUBEM05OHCA0lNaWJHROGpm4T4cc
YQI9JQk/0lB7itF1qK5RG74qgKdjkBkfZxi0OqkUgHk6YHtJlKfET8zfrtcCAwEA
AaNQME4wHQYDVR0OBBYEFGwb0NgH0Zr7Ga23njEJ85Ozf8M9MB8GA1UdIwQYMBaA
FGwb0NgH0Zr7Ga23njEJ85Ozf8M9MAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggIBAKU3RHXggbq/pLhGinU5q/9QT0TB/0bBnF1wNFkKQC0FrNJ+ZnBNmusy
oqOn7DEohBCCDxT0kgOC05gLEsGLkSXlVyqCsPFfycCFhtu1QzSRtQNRxB3pW3Wq
4/RFVYv0PGBjVBKxImQlEmXJWEDwemGKqDQZPtqR/FTHTbJcaT0xQr5+1oG6lawt
I/2cW6GQ0kYW/Szps8FgNdSNgVqCjjNIzBYbWhRWMx/63qD1ReUbY7/Yw9KKT8nK
zXERpbTM9k+Pnm0g9Gep+9HJ1dBFJeuTPugKeSeyqg2OJbENw1hxGs/HjBXw7580
ioiMn/kMj6Tg/f3HCfKrdHHBFQw0/fJW6o17QImYIpPOPzc5RjXBrCJWb34kxqEd
NQdKgejWiV/LlVsguIF8hVZH2kRzvoyypkVUtSUYGmjvA5UXoORQZfJ+b41llq1B
GcSF6iaVbAFKnsUyyr1i9uHz/6Muqflphv/SfZxGheIn5u3PnhXrzDagvItjw0NS
n0Xq64k7fc42HXJpF8CGBkSaIhtlzcruO+vqR80B9r62+D0V7VmHOnP135MT6noU
8F0JQfEtP+I8NII5jHSF/khzSgP5g80LS9tEc2ILnIHK1StkInAoRQQ+/HsQsgbz
ANAf5kxmMsM0zlN2hkxl0H6o7wKlBSw3RI3cjfilXiMWRPJrzlc4
-----END CERTIFICATE-----
`
