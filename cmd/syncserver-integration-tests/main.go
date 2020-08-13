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
	"time"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	// Path to where kafka is installed.
	kafkaDir = test.Defaulting(os.Getenv("KAFKA_DIR"), "kafka")
	// The URI the kafka zookeeper is listening on.
	zookeeperURI = test.Defaulting(os.Getenv("ZOOKEEPER_URI"), "localhost:2181")
	// The URI the kafka server is listening on.
	kafkaURI = test.Defaulting(os.Getenv("KAFKA_URIS"), "localhost:9092")
	// The address the syncserver should listen on.
	syncserverAddr = test.Defaulting(os.Getenv("SYNCSERVER_URI"), "localhost:9876")
	// How long to wait for the syncserver to write the expected output messages.
	// This needs to be high enough to account for the time it takes to create
	// the postgres database tables which can take a while on travis.
	timeoutString = test.Defaulting(os.Getenv("TIMEOUT"), "10s")
	// The name of maintenance database to connect to in order to create the test database.
	postgresDatabase = test.Defaulting(os.Getenv("POSTGRES_DATABASE"), "postgres")
	// Postgres docker container name (for running psql). If not set, psql must be in PATH.
	postgresContainerName = os.Getenv("POSTGRES_CONTAINER")
	// The name of the test database to create.
	testDatabaseName = test.Defaulting(os.Getenv("DATABASE_NAME"), "syncserver_test")
	// The postgres connection config for connecting to the test database.
	testDatabase = test.Defaulting(os.Getenv("DATABASE"), fmt.Sprintf("dbname=%s sslmode=disable binary_parameters=yes", testDatabaseName))
)

const inputTopic = "syncserverInput"
const clientTopic = "clientapiserverOutput"

var exe = test.KafkaExecutor{
	ZookeeperURI:   zookeeperURI,
	KafkaDirectory: kafkaDir,
	KafkaURI:       kafkaURI,
	// Send stdout and stderr to our stderr so that we see error messages from
	// the kafka process.
	OutputWriter: os.Stderr,
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

func createTestUser(database, username, token string) error {
	cmd := exec.Command(
		filepath.Join(filepath.Dir(os.Args[0]), "create-account"),
		"--database", database,
		"--username", username,
		"--token", token,
	)

	// Send stdout and stderr to our stderr so that we see error messages from
	// the create-account process
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// clientEventJSONForOutputRoomEvent parses the given output room event and extracts the 'Event' JSON. It is
// trimmed to the client format and then canonicalised and returned as a string.
// Panics if there are any problems.
func clientEventJSONForOutputRoomEvent(outputRoomEvent string) string {
	var out api.OutputEvent
	if err := json.Unmarshal([]byte(outputRoomEvent), &out); err != nil {
		panic("failed to unmarshal output room event: " + err.Error())
	}
	clientEvs := gomatrixserverlib.ToClientEvents([]gomatrixserverlib.Event{
		out.NewRoomEvent.Event.Event,
	}, gomatrixserverlib.FormatSync)
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

// startSyncServer creates the database and config file needed for the sync server to run and
// then starts the sync server. The Cmd being executed is returned. A channel is also returned,
// which will have any termination errors sent down it, followed immediately by the channel being closed.
func startSyncServer() (*exec.Cmd, chan error) {

	dir, err := ioutil.TempDir("", "syncapi-server-test")
	if err != nil {
		panic(err)
	}

	cfg, _, err := test.MakeConfig(dir, kafkaURI, testDatabase, "localhost", 10000)
	if err != nil {
		panic(err)
	}
	// TODO use the address assigned by the config generator rather than clobbering.
	cfg.Global.ServerName = "localhost"
	cfg.SyncAPI.InternalAPI.Listen = config.HTTPAddress("http://" + syncserverAddr)
	cfg.SyncAPI.InternalAPI.Connect = cfg.SyncAPI.InternalAPI.Listen

	if err := test.WriteConfig(cfg, dir); err != nil {
		panic(err)
	}

	serverArgs := []string{
		"--config", filepath.Join(dir, test.ConfigFile),
	}

	databases := []string{
		testDatabaseName,
	}

	test.InitDatabase(
		postgresDatabase,
		postgresContainerName,
		databases,
	)

	if err := createTestUser(testDatabase, "alice", "@alice:localhost"); err != nil {
		panic(err)
	}
	if err := createTestUser(testDatabase, "bob", "@bob:localhost"); err != nil {
		panic(err)
	}
	if err := createTestUser(testDatabase, "charlie", "@charlie:localhost"); err != nil {
		panic(err)
	}

	cmd, cmdChan := test.CreateBackgroundCommand(
		filepath.Join(filepath.Dir(os.Args[0]), "dendrite-sync-api-server"),
		serverArgs,
	)

	return cmd, cmdChan
}

// prepareKafka creates the topics which will be written to by the tests.
func prepareKafka() {
	err := exe.DeleteTopic(inputTopic)
	if err != nil {
		panic(err)
	}

	if err = exe.CreateTopic(inputTopic); err != nil {
		panic(err)
	}

	err = exe.DeleteTopic(clientTopic)
	if err != nil {
		panic(err)
	}

	if err = exe.CreateTopic(clientTopic); err != nil {
		panic(err)
	}
}

func testSyncServer(syncServerCmdChan chan error, userID, since, want string) {
	fmt.Printf("==TESTING== testSyncServer(%s,%s)\n", userID, since)
	sinceQuery := ""
	if since != "" {
		sinceQuery = "&since=" + since
	}
	req, err := http.NewRequest(
		"GET",
		"http://"+syncserverAddr+"/api/_matrix/client/r0/sync?timeout=100&access_token="+userID+sinceQuery,
		nil,
	)
	if err != nil {
		panic(err)
	}
	testReq := &test.Request{
		Req:              req,
		WantedStatusCode: 200,
		WantedBody:       test.CanonicalJSONInput([]string{want})[0],
	}
	testReq.Run("sync-api", timeout, syncServerCmdChan)
}

func writeToRoomServerLog(indexes ...int) {
	var roomEvents []string
	for _, i := range indexes {
		roomEvents = append(roomEvents, outputRoomEventTestData[i])
	}
	if err := exe.WriteToTopic(inputTopic, test.CanonicalJSONInput(roomEvents)); err != nil {
		panic(err)
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
	// ensure server is dead, only cleaning up so don't care about errors this returns.
	defer cmd.Process.Kill() // nolint: errcheck

	// $ curl -XPOST -d '{}' "http://localhost:8009/_matrix/client/r0/createRoom?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 2"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 3"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"name":"Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	writeToRoomServerLog(
		i0StateRoomCreate, i1StateAliceJoin, i2StatePowerLevels, i3StateJoinRules, i4StateHistoryVisibility,
		i5AliceMsg, i6AliceMsg, i7AliceMsg, i8StateAliceRoomName,
	)

	// Make sure initial sync works TODO: prev_batch
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
						"events": [`+
		clientEventTestData[i0StateRoomCreate]+","+
		clientEventTestData[i1StateAliceJoin]+","+
		clientEventTestData[i2StatePowerLevels]+","+
		clientEventTestData[i3StateJoinRules]+","+
		clientEventTestData[i4StateHistoryVisibility]+","+
		clientEventTestData[i5AliceMsg]+","+
		clientEventTestData[i6AliceMsg]+","+
		clientEventTestData[i7AliceMsg]+","+
		clientEventTestData[i8StateAliceRoomName]+`],
						"limited": true,
						"prev_batch": ""
					}
				}
			},
			"leave": {}
		}
	}`)
	// Make sure alice's rooms don't leak to bob
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
	// Make sure polling with an up-to-date token returns nothing new
	testSyncServer(syncServerCmdChan, "@alice:localhost", "9", `{
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

	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	writeToRoomServerLog(i9StateBobJoin)

	// Make sure alice sees it TODO: prev_batch
	testSyncServer(syncServerCmdChan, "@alice:localhost", "9", `{
		"account_data": {
			"events": []
		},
		"next_batch": "10",
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
						"limited": false,
						"prev_batch": "",
						"events": [`+clientEventTestData[i9StateBobJoin]+`]
					}
				}
			},
			"leave": {}
		}
	}`)

	// Make sure bob sees the room AND all the current room state TODO: history visibility
	testSyncServer(syncServerCmdChan, "@bob:localhost", "9", `{
		"account_data": {
			"events": []
		},
		"next_batch": "10",
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
						"events": [`+
		clientEventTestData[i0StateRoomCreate]+","+
		clientEventTestData[i1StateAliceJoin]+","+
		clientEventTestData[i2StatePowerLevels]+","+
		clientEventTestData[i3StateJoinRules]+","+
		clientEventTestData[i4StateHistoryVisibility]+","+
		clientEventTestData[i8StateAliceRoomName]+`]
					},
					"timeline": {
						"limited": false,
						"prev_batch": "",
						"events": [`+
		clientEventTestData[i9StateBobJoin]+`]
					}
				}
			},
			"leave": {}
		}
	}`)

	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello alice"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@bob:localhost"
	writeToRoomServerLog(i10BobMsg)

	// Make sure alice can see everything around the join point for bob TODO: prev_batch
	testSyncServer(syncServerCmdChan, "@alice:localhost", "7", `{
		"account_data": {
			"events": []
		},
		"next_batch": "11",
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
						"limited": false,
						"prev_batch": "",
						"events": [`+
		clientEventTestData[i7AliceMsg]+","+
		clientEventTestData[i8StateAliceRoomName]+","+
		clientEventTestData[i9StateBobJoin]+","+
		clientEventTestData[i10BobMsg]+`]
					}
				}
			},
			"leave": {}
		}
	}`)

	// $ curl -XPUT -d '{"name":"A Different Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello bob"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"invite"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@bob:localhost"
	writeToRoomServerLog(i11StateAliceRoomName, i12AliceMsg, i13StateBobInviteCharlie)

	// Make sure charlie sees the invite both with and without a ?since= token
	// TODO: Invite state should include the invite event and the room name.
	charlieInviteData := `{
		"account_data": {
			"events": []
		},
		"next_batch": "14",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {
				"!PjrbIMW2cIiaYF4t:localhost": {
					"invite_state": {
						"events": []
					}
				}
			},
			"join": {},
			"leave": {}
		}
	}`
	testSyncServer(syncServerCmdChan, "@charlie:localhost", "7", charlieInviteData)
	testSyncServer(syncServerCmdChan, "@charlie:localhost", "", charlieInviteData)

	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@charlie:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"not charlie..."}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@alice:localhost"
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"why did you kick charlie"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@bob:localhost"
	writeToRoomServerLog(i14StateCharlieJoin, i15AliceMsg, i16StateAliceKickCharlie, i17BobMsg)

	// Check transitions to leave work
	testSyncServer(syncServerCmdChan, "@charlie:localhost", "15", `{
		"account_data": {
			"events": []
		},
		"next_batch": "18",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {},
			"join": {},
			"leave": {
				"!PjrbIMW2cIiaYF4t:localhost": {
					"state": {
						"events": []
					},
					"timeline": {
						"limited": false,
						"prev_batch": "",
						"events": [`+
		clientEventTestData[i15AliceMsg]+","+
		clientEventTestData[i16StateAliceKickCharlie]+`]
					}
				}
			}
		}
	}`)

	// Test joining and leaving the same room in a single /sync request puts the room in the 'leave' section.
	// TODO: Use an earlier since value to assert that the /sync response doesn't leak messages
	//       from before charlie was joined to the room. Currently it does leak because RecentEvents doesn't
	//       take membership into account.
	testSyncServer(syncServerCmdChan, "@charlie:localhost", "14", `{
		"account_data": {
			"events": []
		},
		"next_batch": "18",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {},
			"join": {},
			"leave": {
				"!PjrbIMW2cIiaYF4t:localhost": {
					"state": {
						"events": []
					},
					"timeline": {
						"limited": false,
						"prev_batch": "",
						"events": [`+
		clientEventTestData[i14StateCharlieJoin]+","+
		clientEventTestData[i15AliceMsg]+","+
		clientEventTestData[i16StateAliceKickCharlie]+`]
					}
				}
			}
		}
	}`)

	// $ curl -XPUT -d '{"name":"No Charlies"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	writeToRoomServerLog(i18StateAliceRoomName)

	// Check that users don't see state changes in rooms after they have left
	testSyncServer(syncServerCmdChan, "@charlie:localhost", "17", `{
		"account_data": {
			"events": []
		},
		"next_batch": "19",
		"presence": {
			"events": []
		},
		"rooms": {
			"invite": {},
			"join": {},
			"leave": {}
		}
	}`)

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
