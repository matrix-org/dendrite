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
version: 0
matrix:
  server_name: localhost
  private_key: matrix_key.pem
  federation_certificates: [tls_cert.pem]
media:
  base_path: media_store
kafka:
  addresses: ["localhost:9092"]
  topics:
    output_room_event: output.room
    output_client_data: output.client
    output_typing_event: output.typing
    user_updates: output.user
database:
  media_api: "postgresql:///media_api"
  account: "postgresql:///account"
  device: "postgresql:///device"
  server_key: "postgresql:///server_keys"
  sync_api: "postgresql:///syn_api"
  room_server: "postgresql:///room_server"
  appservice: "postgresql:///appservice"
  current_state: "postgresql:///current_state"
listen:
  room_server: "localhost:7770"
  client_api: "localhost:7771"
  federation_api: "localhost:7772"
  sync_api: "localhost:7773"
  media_api: "localhost:7774"
  appservice_api: "localhost:7777"
  edu_server: "localhost:7778"
  user_api: "localhost:7779"
  current_state_server: "localhost:7775"
logging:
  - type: "file"
    level: "info"
    params:
      path: "/my/log/dir"
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
