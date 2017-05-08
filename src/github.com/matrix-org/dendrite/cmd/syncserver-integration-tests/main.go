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

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common/test"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	// Path to where kafka is installed.
	kafkaDir = defaulting(os.Getenv("KAFKA_DIR"), "kafka")
	// The URI the kafka zookeeper is listening on.
	zookeeperURI = defaulting(os.Getenv("ZOOKEEPER_URI"), "localhost:2181")
	// The URI the kafka server is listening on.
	kafkaURI = defaulting(os.Getenv("KAFKA_URIS"), "localhost:9092")
	// The address the syncserver should listen on.
	syncserverAddr = defaulting(os.Getenv("SYNCSERVER_URI"), "localhost:9876")
	// How long to wait for the syncserver to write the expected output messages.
	// This needs to be high enough to account for the time it takes to create
	// the postgres database tables which can take a while on travis.
	timeoutString = defaulting(os.Getenv("TIMEOUT"), "10s")
	// The name of maintenance database to connect to in order to create the test database.
	postgresDatabase = defaulting(os.Getenv("POSTGRES_DATABASE"), "postgres")
	// The name of the test database to create.
	testDatabaseName = defaulting(os.Getenv("DATABASE_NAME"), "syncserver_test")
	// The postgres connection config for connecting to the test database.
	testDatabase = defaulting(os.Getenv("DATABASE"), fmt.Sprintf("dbname=%s sslmode=disable binary_parameters=yes", testDatabaseName))
)

const inputTopic = "syncserverInput"

var env = test.KafkaEnv{
	ZookeeperURI:   zookeeperURI,
	KafkaDirectory: kafkaDir,
	KafkaURI:       kafkaURI,
	// Send stdout and stderr to our stderr so that we see error messages from
	// the kafka process.
	OutputWriter: os.Stderr,
}

var syncServerConfigFileContents = (`consumer_uris: ["` + kafkaURI + `"]
roomserver_topic: "` + inputTopic + `"
database: "` + testDatabase + `"
`)

func defaulting(value, defaultValue string) string {
	if value == "" {
		value = defaultValue
	}
	return value
}

var timeout time.Duration

func init() {
	var err error
	timeout, err = time.ParseDuration(timeoutString)
	if err != nil {
		panic(err)
	}
}

// TODO: dupes roomserver integration tests. Factor out.
func createDatabase(database string) error {
	cmd := exec.Command("psql", postgresDatabase)
	cmd.Stdin = strings.NewReader(
		fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s;", database, database),
	)
	// Send stdout and stderr to our stderr so that we see error messages from
	// the psql process
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// TODO: dupes roomserver integration tests. Factor out.
func canonicalJSONInput(jsonData []string) []string {
	for i := range jsonData {
		jsonBytes, err := gomatrixserverlib.CanonicalJSON([]byte(jsonData[i]))
		if err != nil {
			panic(err)
		}
		jsonData[i] = string(jsonBytes)
	}
	return jsonData
}

func doSyncRequest(done chan error, want []string, since string) func() {
	return func() {
		cli := &http.Client{
			Timeout: 5 * time.Second,
		}
		res, err := cli.Get("http://" + syncserverAddr + "/api/_matrix/client/r0/sync?access_token=@alice:localhost&since=" + since)
		if err != nil {
			done <- err
			return
		}
		if res.StatusCode != 200 {
			done <- fmt.Errorf("/sync returned HTTP status %d", res.StatusCode)
			return
		}
		defer res.Body.Close()
		resBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			done <- err
			return
		}
		jsonBytes, err := gomatrixserverlib.CanonicalJSON(resBytes)
		if err != nil {
			done <- err
			return
		}
		if string(jsonBytes) != want[0] {
			fmt.Println("Expected:")
			fmt.Println(want[0])
			fmt.Println("Got:")
			fmt.Println(string(jsonBytes))
			done <- fmt.Errorf("/sync returned wrong bytes")
			return
		}
		// all good, clean up
		close(done)
	}
}

func testSyncServer(input, want []string, since string) {
	env.DeleteTopic(inputTopic)
	if err := env.CreateTopic(inputTopic); err != nil {
		panic(err)
	}
	if err := env.WriteToTopic(inputTopic, canonicalJSONInput(input)); err != nil {
		panic(err)
	}

	if err := createDatabase(testDatabaseName); err != nil {
		panic(err)
	}

	const configFileName = "sync-api-server-config-test.yaml"
	err := ioutil.WriteFile(configFileName, []byte(syncServerConfigFileContents), 0644)
	if err != nil {
		panic(err)
	}

	cmd := exec.Command(
		filepath.Join(filepath.Dir(os.Args[0]), "dendrite-sync-api-server"),
		"--config", configFileName,
		"--listen", syncserverAddr,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	if err := cmd.Start(); err != nil {
		panic("failed to start sync server: " + err.Error())
	}
	done := make(chan error, 1)

	// TODO: Waiting 1s is racey. Maybe keep hitting it until it doesn't get Connection Refused?
	time.AfterFunc(1*time.Second, doSyncRequest(done, want, since))

	// wait for it to die or timeout
	go func() {
		cmdErr := cmd.Wait()
		if cmdErr != nil {
			exitErr, ok := cmdErr.(*exec.ExitError)
			if ok {
				fmt.Println("\nSYNC SERVER ERROR: (", exitErr, ")")
				fmt.Println("sync server failed to run. If failing with 'pq: password authentication failed for user' try:")
				fmt.Println("    export PGHOST=/var/run/postgresql\n")
			}
		}
		done <- cmdErr
	}()
	select {
	case <-time.After(timeout):
		if err := cmd.Process.Kill(); err != nil {
			panic(err)
		}
		panic("dendrite-sync-api-server timed out")
	case err := <-done:
		cmd.Process.Kill() // ensure server is dead, only cleaning up so don't care about errors this returns.
		if err != nil {
			panic(err)
		}
	}
}

func main() {
	fmt.Println("==TESTING==", os.Args[0])
	// room creation for @alice:localhost
	input := []string{
		`{
		"Event": {
			"auth_events": [],
			"content": {
				"creator": "@alice:localhost"
			},
			"depth": 1,
			"event_id": "$rOaxKSu6K1s0nOsW:localhost",
			"hashes": {
				"sha256": "g1QC1jZauIcVw+HCGizUqlUaLSmAkEGwGmIcLac5TKk"
			},
			"origin": "localhost",
			"origin_server_ts": 1493908927170,
			"prev_events": [],
			"room_id": "!gnrFfNAK7yGBWXFd:localhost",
			"sender": "@alice:localhost",
			"signatures": {
				"localhost": {
					"ed25519:something": "WCaImDmpkhNCCoUyRHcrV93SeJpJbq34yWbtjBgNNXVJaoiLSTys6t/gCvVqNYfX6Dt9c+z/sx5LikOLmLm1Dg"
				}
			},
			"state_key": "",
			"type": "m.room.create"
		},
		"VisibilityEventIDs": null,
		"LatestEventIDs": ["$rOaxKSu6K1s0nOsW:localhost"],
		"AddsStateEventIDs": ["$rOaxKSu6K1s0nOsW:localhost"],
		"RemovesStateEventIDs": null,
		"LastSentEventID": ""
	}`,
		`{
		"Event": {
			"auth_events": [
				["$rOaxKSu6K1s0nOsW:localhost", {
					"sha256": "XFb+VOx/74T3RPw2PXTY4AXDZaEy8uLCSFuHCK4XYHg"
				}]
			],
			"content": {
				"membership": "join"
			},
			"depth": 2,
			"event_id": "$uEDYwFpBO936HTfM:localhost",
			"hashes": {
				"sha256": "y5AQAnnzremC678QTIFEi677wdbMwluPiweZnuvUmz0"
			},
			"origin": "localhost",
			"origin_server_ts": 1493908927170,
			"prev_events": [
				["$rOaxKSu6K1s0nOsW:localhost", {
					"sha256": "XFb+VOx/74T3RPw2PXTY4AXDZaEy8uLCSFuHCK4XYHg"
				}]
			],
			"room_id": "!gnrFfNAK7yGBWXFd:localhost",
			"sender": "@alice:localhost",
			"signatures": {
				"localhost": {
					"ed25519:something": "5Pl8GkgcyUu2QY7T38OkuufVQQV13f0kl2PLFI2OILBIcy0XPf8hSaFclemYckoo2nRgffIzsHO/ZgqfoBu0BA"
				}
			},
			"state_key": "@alice:localhost",
			"type": "m.room.member"
		},
		"VisibilityEventIDs": null,
		"LatestEventIDs": ["$uEDYwFpBO936HTfM:localhost"],
		"AddsStateEventIDs": ["$uEDYwFpBO936HTfM:localhost"],
		"RemovesStateEventIDs": null,
		"LastSentEventID": "$rOaxKSu6K1s0nOsW:localhost"
	}`,
		`{
		"Event": {
			"auth_events": [
				["$rOaxKSu6K1s0nOsW:localhost", {
					"sha256": "XFb+VOx/74T3RPw2PXTY4AXDZaEy8uLCSFuHCK4XYHg"
				}],
				["$uEDYwFpBO936HTfM:localhost", {
					"sha256": "3z+JL3VmTtVROucpsrEWkxNVzn8ZOP2I1jU362pQIUU"
				}]
			],
			"content": {
				"ban": 50,
				"events": {
					"m.room.avatar": 50,
					"m.room.canonical_alias": 50,
					"m.room.history_visibility": 100,
					"m.room.name": 50,
					"m.room.power_levels": 100
				},
				"events_default": 0,
				"invite": 0,
				"kick": 50,
				"redact": 50,
				"state_default": 50,
				"users": {
					"@alice:localhost": 100
				},
				"users_default": 0
			},
			"depth": 3,
			"event_id": "$Axp7qdQXf0bz7zBy:localhost",
			"hashes": {
				"sha256": "oObDsGkeVtQgyVPauoLIqk+J+Jsz6HOol79uRMTRFFM"
			},
			"origin": "localhost",
			"origin_server_ts": 1493908927171,
			"prev_events": [
				["$uEDYwFpBO936HTfM:localhost", {
					"sha256": "3z+JL3VmTtVROucpsrEWkxNVzn8ZOP2I1jU362pQIUU"
				}]
			],
			"room_id": "!gnrFfNAK7yGBWXFd:localhost",
			"sender": "@alice:localhost",
			"signatures": {
				"localhost": {
					"ed25519:something": "3kV1Wm2E1zUPQ8YUIC1x/8ks1SGvXE0olQ+b0BRMJm7fduY2fNcb/4A4aKbQLRtOwvCNUVuqQkkkdp1Zor1LCw"
				}
			},
			"state_key": "",
			"type": "m.room.power_levels"
		},
		"VisibilityEventIDs": null,
		"LatestEventIDs": ["$Axp7qdQXf0bz7zBy:localhost"],
		"AddsStateEventIDs": ["$Axp7qdQXf0bz7zBy:localhost"],
		"RemovesStateEventIDs": null,
		"LastSentEventID": "$uEDYwFpBO936HTfM:localhost"
	}`,
		`{
		"Event": {
			"auth_events": [
				["$rOaxKSu6K1s0nOsW:localhost", {
					"sha256": "XFb+VOx/74T3RPw2PXTY4AXDZaEy8uLCSFuHCK4XYHg"
				}],
				["$Axp7qdQXf0bz7zBy:localhost", {
					"sha256": "5KIh9uRcgXuiYdO965JSfIOSGeMrasf8N9eEzxisErI"
				}],
				["$uEDYwFpBO936HTfM:localhost", {
					"sha256": "3z+JL3VmTtVROucpsrEWkxNVzn8ZOP2I1jU362pQIUU"
				}]
			],
			"content": {
				"join_rule": "public"
			},
			"depth": 4,
			"event_id": "$zCgCrw3aZwVaKm34:localhost",
			"hashes": {
				"sha256": "KmJ7wAUznMy74MhAB3iDsBdFAkGypWXamDDQeLVzp1w"
			},
			"origin": "localhost",
			"origin_server_ts": 1493908927172,
			"prev_events": [
				["$Axp7qdQXf0bz7zBy:localhost", {
					"sha256": "5KIh9uRcgXuiYdO965JSfIOSGeMrasf8N9eEzxisErI"
				}]
			],
			"room_id": "!gnrFfNAK7yGBWXFd:localhost",
			"sender": "@alice:localhost",
			"signatures": {
				"localhost": {
					"ed25519:something": "BkqU/1QARxNWEDfgKenvrhhGd6nmNZYHugHB0kFqUSQRZo+RV/zThLA0FxMXfmbGqfJdi1wXmxIR3QIwvGuhCg"
				}
			},
			"state_key": "",
			"type": "m.room.join_rules"
		},
		"VisibilityEventIDs": null,
		"LatestEventIDs": ["$zCgCrw3aZwVaKm34:localhost"],
		"AddsStateEventIDs": ["$zCgCrw3aZwVaKm34:localhost"],
		"RemovesStateEventIDs": null,
		"LastSentEventID": "$Axp7qdQXf0bz7zBy:localhost"
	}`,
		`{
		"Event": {
			"auth_events": [
				["$rOaxKSu6K1s0nOsW:localhost", {
					"sha256": "XFb+VOx/74T3RPw2PXTY4AXDZaEy8uLCSFuHCK4XYHg"
				}],
				["$Axp7qdQXf0bz7zBy:localhost", {
					"sha256": "5KIh9uRcgXuiYdO965JSfIOSGeMrasf8N9eEzxisErI"
				}],
				["$uEDYwFpBO936HTfM:localhost", {
					"sha256": "3z+JL3VmTtVROucpsrEWkxNVzn8ZOP2I1jU362pQIUU"
				}]
			],
			"content": {
				"history_visibility": "joined"
			},
			"depth": 5,
			"event_id": "$0NUtdnY7KWMhOR9E:localhost",
			"hashes": {
				"sha256": "9CBp3jcnGKzoKCVYRCFCoe0CJ8IfZZAOhudAoDr2jqU"
			},
			"origin": "localhost",
			"origin_server_ts": 1493908927174,
			"prev_events": [
				["$zCgCrw3aZwVaKm34:localhost", {
					"sha256": "8kNj8j5K6YFWpFa0CLy1pR5Lp9nao0X6TW2iUIya2Tc"
				}]
			],
			"room_id": "!gnrFfNAK7yGBWXFd:localhost",
			"sender": "@alice:localhost",
			"signatures": {
				"localhost": {
					"ed25519:something": "92Dz7JXAxuc87L3+jMps0HC6Z4V5PhMZQIomI8Dod/im1bkfhYUPMOF5EWWMGMDSq+mSpJPVizWAIGa8bIFcDA"
				}
			},
			"state_key": "",
			"type": "m.room.history_visibility"
		},
		"VisibilityEventIDs": null,
		"LatestEventIDs": ["$0NUtdnY7KWMhOR9E:localhost"],
		"AddsStateEventIDs": ["$0NUtdnY7KWMhOR9E:localhost"],
		"RemovesStateEventIDs": null,
		"LastSentEventID": "$zCgCrw3aZwVaKm34:localhost"
	}`,
	}
	since := "3"
	want := []string{
		`{
		"next_batch": "5",
		"account_data": {
			"events": []
		},
		"presence": {
			"events": []
		},
		"rooms": {
			"join": {
				"!gnrFfNAK7yGBWXFd:localhost": {
					"state": {
						"events": [{
							"content": {
								"join_rule": "public"
							},
							"event_id": "$zCgCrw3aZwVaKm34:localhost",
							"origin_server_ts": 1493908927172,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.join_rules"
						}]
					},
					"timeline": {
						"events": [{
							"content": {
								"join_rule": "public"
							},
							"event_id": "$zCgCrw3aZwVaKm34:localhost",
							"origin_server_ts": 1493908927172,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.join_rules"
						}, {
							"content": {
								"history_visibility": "joined"
							},
							"event_id": "$0NUtdnY7KWMhOR9E:localhost",
							"origin_server_ts": 1493908927174,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.history_visibility"
						}],
						"limited": false,
						"prev_batch": ""
					},
					"ephemeral": {
						"events": []
					},
					"account_data": {
						"events": []
					}
				}
			},
			"invite": {},
			"leave": {}
		}
	}`,
	}
	want = canonicalJSONInput(want)
	testSyncServer(input, want, since)
}
