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
version: 2
global:
  server_name: localhost
  private_key: matrix_key.pem
  key_id: ed25519:auto
  key_validity_period: 168h0m0s
  well_known_server_name: "localhost:443"
  trusted_third_party_id_servers:
  - matrix.org
  - vector.im
  kafka:
    addresses:
    - localhost:2181
    topic_prefix: Dendrite
    use_naffka: true
    naffka_database:
      connection_string: file:naffka.db
      max_open_conns: 100
      max_idle_conns: 2
      conn_max_lifetime: -1
  metrics:
    enabled: false
    basic_auth:
      username: metrics
      password: metrics
  server_notices:
    local_part: "_server"
    display_name: "Server alerts"
    avatar: ""
    room_name: "Server Alerts"	
app_service_api:
  internal_api:
    listen: http://localhost:7777
    connect: http://localhost:7777
  database:
    connection_string: file:appservice.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  config_files: []
client_api:
  internal_api:
    listen: http://localhost:7771
    connect: http://localhost:7771
  external_api:
    listen: http://[::]:8071
  registration_disabled: false
  registration_shared_secret: ""
  enable_registration_captcha: false
  recaptcha_public_key: ""
  recaptcha_private_key: ""
  recaptcha_bypass_secret: ""
  recaptcha_siteverify_api: ""
  turn:
    turn_user_lifetime: ""
    turn_uris: []
    turn_shared_secret: ""
    turn_username: ""
    turn_password: ""
current_state_server:
  internal_api:
    listen: http://localhost:7782
    connect: http://localhost:7782
  database:
    connection_string: file:currentstate.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
edu_server:
  internal_api:
    listen: http://localhost:7778
    connect: http://localhost:7778
federation_api:
  internal_api:
    listen: http://localhost:7772
    connect: http://localhost:7772
  external_api:
    listen: http://[::]:8072
  federation_certificates: []
federation_sender:
  internal_api:
    listen: http://localhost:7775
    connect: http://localhost:7775
  database:
    connection_string: file:federationapi.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  send_max_retries: 16
  disable_tls_validation: false
key_server:
  internal_api:
    listen: http://localhost:7779
    connect: http://localhost:7779
  database:
    connection_string: file:keyserver.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
media_api:
  internal_api:
    listen: http://localhost:7774
    connect: http://localhost:7774
  external_api:
    listen: http://[::]:8074
  database:
    connection_string: file:mediaapi.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  base_path: ./media_store
  max_file_size_bytes: 10485760
  dynamic_thumbnails: false
  max_thumbnail_generators: 10
  thumbnail_sizes:
  - width: 32
    height: 32
    method: crop
  - width: 96
    height: 96
    method: crop
  - width: 640
    height: 480
    method: scale
room_server:
  internal_api:
    listen: http://localhost:7770
    connect: http://localhost:7770
  database:
    connection_string: file:roomserver.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
server_key_api:
  internal_api:
    listen: http://localhost:7780
    connect: http://localhost:7780
  database:
    connection_string: file:serverkeyapi.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  key_perspectives:
  - server_name: matrix.org
    keys:
    - key_id: ed25519:auto
      public_key: Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw
    - key_id: ed25519:a_RXGa
      public_key: l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ
sync_api:
  internal_api:
    listen: http://localhost:7773
    connect: http://localhost:7773
  database:
    connection_string: file:syncapi.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
user_api:
  internal_api:
    listen: http://localhost:7781
    connect: http://localhost:7781
  account_database:
    connection_string: file:userapi_accounts.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  device_database:
    connection_string: file:userapi_devices.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
  pusher_database:
    connection_string: file:pushserver.db
    max_open_conns: 100
    max_idle_conns: 2
    conn_max_lifetime: -1
tracing:
  enabled: false
  jaeger:
    serviceName: ""
    disabled: false
    rpc_metrics: false
    tags: []
    sampler: null
    reporter: null
    headers: null
    baggage_restrictions: null
    throttler: null
logging:
- type: file
  level: info
  params:
    path: /var/log/dendrite
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
	keyID, _, err := readKeyPEM("path/to/key", []byte(testKey), true)
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
