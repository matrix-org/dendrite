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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/common/test"
	"github.com/matrix-org/dendrite/roomserver/api"
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

var exe = test.KafkaExecutor{
	ZookeeperURI:   zookeeperURI,
	KafkaDirectory: kafkaDir,
	KafkaURI:       kafkaURI,
	// Send stdout and stderr to our stderr so that we see error messages from
	// the kafka process.
	OutputWriter: os.Stderr,
}

var (
	lastRequestMutex sync.Mutex
	lastRequestErr   error
)

func setLastRequestError(err error) {
	lastRequestMutex.Lock()
	defer lastRequestMutex.Unlock()
	lastRequestErr = err
}

func getLastRequestError() error {
	lastRequestMutex.Lock()
	defer lastRequestMutex.Unlock()
	return lastRequestErr
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
var clientEventTestData []string

func init() {
	var err error
	timeout, err = time.ParseDuration(timeoutString)
	if err != nil {
		panic(err)
	}

	for _, s := range outputRoomEventTestData {
		clientEventTestData = append(clientEventTestData, clientEventJSONForOutputRoomEvent(s))
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

// clientEventJSONForOutputRoomEvent parses the given output room event and extracts the 'Event' JSON. It is
// trimmed to the client format and then canonicalised and returned as a string.
// Panics if there are any problems.
func clientEventJSONForOutputRoomEvent(outputRoomEvent string) string {
	var out api.OutputRoomEvent
	if err := json.Unmarshal([]byte(outputRoomEvent), &out); err != nil {
		panic("failed to unmarshal output room event: " + err.Error())
	}
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON(out.Event, false)
	if err != nil {
		panic("failed to convert event field in output room event to Event: " + err.Error())
	}
	clientEvs := gomatrixserverlib.ToClientEvents([]gomatrixserverlib.Event{ev}, gomatrixserverlib.FormatSync)
	b, err := json.Marshal(clientEvs[0])
	if err != nil {
		panic("failed to marshal client event as json: " + err.Error())
	}
	jsonBytes, err := gomatrixserverlib.CanonicalJSON(b)
	if err != nil {
		panic("failed to turn event json into canonical json: " + err.Error())
	}
	return string(jsonBytes)
}

// doSyncRequest does a /sync request and returns an error if it fails or doesn't
// return the wanted string.
func doSyncRequest(syncServerURL, want string) error {
	cli := &http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := cli.Get(syncServerURL)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("/sync returned HTTP status %d", res.StatusCode)
	}
	defer res.Body.Close()
	resBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	jsonBytes, err := gomatrixserverlib.CanonicalJSON(resBytes)
	if err != nil {
		return err
	}
	if string(jsonBytes) != want {
		return fmt.Errorf("/sync returned wrong bytes. Expected:\n%s\n\nGot:\n%s", want, string(jsonBytes))
	}
	return nil
}

// syncRequestUntilSuccess blocks and performs the same /sync request over and over until
// the response returns the wanted string, where it will close the given channel and return.
// It will keep track of the last error in `lastRequestErr`.
func syncRequestUntilSuccess(done chan error, userID, since, want string) {
	for {
		sinceQuery := ""
		if since != "" {
			sinceQuery = "&since=" + since
		}
		err := doSyncRequest(
			"http://"+syncserverAddr+"/api/_matrix/client/r0/sync?access_token="+userID+sinceQuery,
			want,
		)
		if err != nil {
			setLastRequestError(err)
			time.Sleep(1 * time.Second) // don't tightloop
			continue
		}
		close(done)
		return
	}
}

// startSyncServer creates the database and config file needed for the sync server to run and
// then starts the sync server. The Cmd being executed is returned. A channel is also returned,
// which will have any termination errors sent down it, followed immediately by the channel being closed.
func startSyncServer() (*exec.Cmd, chan error) {
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
	syncServerCmdChan := make(chan error, 1)
	go func() {
		syncServerCmdChan <- cmd.Wait()
		close(syncServerCmdChan)
	}()
	return cmd, syncServerCmdChan
}

// prepareKafka creates the topics which will be written to by the tests.
func prepareKafka() {
	exe.DeleteTopic(inputTopic)
	if err := exe.CreateTopic(inputTopic); err != nil {
		panic(err)
	}
}

func testSyncServer(syncServerCmdChan chan error, userID, since, want string) {
	fmt.Printf("==TESTING== testSyncServer(%s,%s)\n", userID, since)
	done := make(chan error, 1)

	// We need to wait for the sync server to:
	// - have created the tables
	// - be listening on the given port
	// - have consumed the kafka logs
	// before we begin hitting it with /sync requests. We don't get told when it has done
	// all these things, so we just continually hit /sync until it returns the right bytes.
	// We can't even wait for the first valid 200 OK response because it's possible to race
	// with consuming the kafka logs (so the /sync response will be missing events and
	// therefore fail the test).
	go syncRequestUntilSuccess(done, userID, since, canonicalJSONInput([]string{want})[0])

	// wait for one of:
	// - the test to pass (done channel is closed)
	// - the sync server to exit with an error (error sent on syncServerCmdChan)
	// - our test timeout to expire
	// We don't need to clean up since the main() function handles that in the event we panic
	var testPassed bool

	select {
	case <-time.After(timeout):
		if testPassed {
			break
		}
		fmt.Printf("==TESTING== testSyncServer(%s,%s) TIMEOUT\n", userID, since)
		if reqErr := getLastRequestError(); reqErr != nil {
			fmt.Println("Last /sync request error:")
			fmt.Println(reqErr)
		}
		panic("dendrite-sync-api-server timed out")
	case err := <-syncServerCmdChan:
		if err != nil {
			fmt.Println("=============================================================================================")
			fmt.Println("sync server failed to run. If failing with 'pq: password authentication failed for user' try:")
			fmt.Println("    export PGHOST=/var/run/postgresql\n")
			fmt.Println("=============================================================================================")
			panic(err)
		}
	case <-done:
		testPassed = true
		fmt.Printf("==TESTING== testSyncServer(%s,%s) PASSED\n", userID, since)
	}
}

// Runs a battery of sync server tests against test data in testdata.go
// testdata.go has a list of OutputRoomEvents which will be fed into the kafka log which the sync server will consume.
// The tests will pause at various points in this list to conduct tests on the /sync responses before continuing.
// For ease of understanding, the curl commands used to create the OutputRoomEvents are listed along with each write to kafka.
func main() {
	fmt.Println("==TESTING==", os.Args[0])
	prepareKafka()
	cmd, syncServerCmdChan := startSyncServer()
	defer cmd.Process.Kill() // ensure server is dead, only cleaning up so don't care about errors this returns.

	// $ curl -XPOST -d '{}' "http://localhost:8009/_matrix/client/r0/createRoom?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 2"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 3"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"name":"Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	if err := exe.WriteToTopic(inputTopic, canonicalJSONInput(outputRoomEventTestData[0:9])); err != nil {
		panic(err)
	}
	testSyncServer(syncServerCmdChan, "@alice:localhost", "", `{
		"account_data": {
			"events": []
		},
		"next_batch": "9",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {},
			"join": {
				"!PjrbIMW2cIiaYF4t:localhost": {
					"account_data": {
						"events": []
					},
					"ephemeral": {
						"events": []
					},
					"state": {
						"events": []
					},
					"timeline": {
						"events": [{
							"content": {
								"creator": "@alice:localhost"
							},
							"event_id": "$xz0fUB8zNMTGFh1W:localhost",
							"origin_server_ts": 1494411218382,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.create"
						}, {
							"content": {
								"membership": "join"
							},
							"event_id": "$QTen1vksfcRTpUCk:localhost",
							"origin_server_ts": 1494411218385,
							"sender": "@alice:localhost",
							"state_key": "@alice:localhost",
							"type": "m.room.member"
						}, {
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
							"event_id": "$RWsxGlfPHAcijTgu:localhost",
							"origin_server_ts": 1494411218385,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.power_levels"
						}, {
							"content": {
								"join_rule": "public"
							},
							"event_id": "$2O2DpHB37CuwwJOe:localhost",
							"origin_server_ts": 1494411218386,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.join_rules"
						}, {
							"content": {
								"history_visibility": "joined"
							},
							"event_id": "$5LRiBskVCROnL5WY:localhost",
							"origin_server_ts": 1494411218387,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.history_visibility"
						}, {
							"content": {
								"body": "hello world",
								"msgtype": "m.text"
							},
							"event_id": "$Z8ZJik7ghwzSYTH9:localhost",
							"origin_server_ts": 1494411339207,
							"sender": "@alice:localhost",
							"type": "m.room.message"
						}, {
							"content": {
								"body": "hello world 2",
								"msgtype": "m.text"
							},
							"event_id": "$8382Ah682eL4hxjN:localhost",
							"origin_server_ts": 1494411380282,
							"sender": "@alice:localhost",
							"type": "m.room.message"
						}, {
							"content": {
								"body": "hello world 3",
								"msgtype": "m.text"
							},
							"event_id": "$17SfHsvSeTQthSWF:localhost",
							"origin_server_ts": 1494411396560,
							"sender": "@alice:localhost",
							"type": "m.room.message"
						}, {
							"content": {
								"name": "Custom Room Name"
							},
							"event_id": "$j7KtuOzM0K15h3Kr:localhost",
							"origin_server_ts": 1494411482625,
							"sender": "@alice:localhost",
							"state_key": "",
							"type": "m.room.name"
						}],
						"limited": true,
						"prev_batch": ""
					}
				}
			},
			"leave": {}
		}
	}`)
	testSyncServer(syncServerCmdChan, "@bob:localhost", "", `{
		"account_data": {
			"events": []
		},
		"next_batch": "9",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {},
			"join": {},
			"leave": {}
		}
	}`)

	// TODO: Add more tests
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello alice"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"name":"A Different Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello bob"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"invite"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@charlie:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"not charlie..."}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"why did you kick charlie"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"name":"No Charlies"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"whatever"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"im alone now"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"invite"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"so alone"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"name":"Everyone welcome"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@charlie:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hiiiii"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@charlie:localhost"
}
