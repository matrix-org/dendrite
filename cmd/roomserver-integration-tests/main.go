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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"

	"net/http"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/inthttp"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	// Path to where kafka is installed.
	kafkaDir = defaulting(os.Getenv("KAFKA_DIR"), "kafka")
	// The URI the kafka zookeeper is listening on.
	zookeeperURI = defaulting(os.Getenv("ZOOKEEPER_URI"), "localhost:2181")
	// The URI the kafka server is listening on.
	kafkaURI = defaulting(os.Getenv("KAFKA_URIS"), "localhost:9092")
	// How long to wait for the roomserver to write the expected output messages.
	// This needs to be high enough to account for the time it takes to create
	// the postgres database tables which can take a while on travis.
	timeoutString = defaulting(os.Getenv("TIMEOUT"), "60s")
	// Timeout for http client
	timeoutHTTPClient = defaulting(os.Getenv("TIMEOUT_HTTP"), "30s")
	// The name of maintenance database to connect to in order to create the test database.
	postgresDatabase = defaulting(os.Getenv("POSTGRES_DATABASE"), "postgres")
	// The name of the test database to create.
	testDatabaseName = defaulting(os.Getenv("DATABASE_NAME"), "roomserver_test")
	// The postgres connection config for connecting to the test database.
	testDatabase = defaulting(os.Getenv("DATABASE"), fmt.Sprintf("dbname=%s binary_parameters=yes", testDatabaseName))
)

var exe = test.KafkaExecutor{
	ZookeeperURI:   zookeeperURI,
	KafkaDirectory: kafkaDir,
	KafkaURI:       kafkaURI,
	// Send stdout and stderr to our stderr so that we see error messages from
	// the kafka process.
	OutputWriter: os.Stderr,
}

func defaulting(value, defaultValue string) string {
	if value == "" {
		value = defaultValue
	}
	return value
}

var (
	timeout     time.Duration
	timeoutHTTP time.Duration
)

func init() {
	var err error
	timeout, err = time.ParseDuration(timeoutString)
	if err != nil {
		panic(err)
	}
	timeoutHTTP, err = time.ParseDuration(timeoutHTTPClient)
	if err != nil {
		panic(err)
	}
}

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

// runAndReadFromTopic runs a command and waits for a number of messages to be
// written to a kafka topic. It returns if the command exits, the number of
// messages is reached or after a timeout. It kills the command before it returns.
// It returns a list of the messages read from the command on success or an error
// on failure.
func runAndReadFromTopic(runCmd *exec.Cmd, readyURL string, doInput func(), topic string, count int, checkQueryAPI func()) ([]string, error) {
	type result struct {
		// data holds all of stdout on success.
		data []byte
		// err is set on failure.
		err error
	}
	done := make(chan result)
	readCmd := exec.Command(
		filepath.Join(kafkaDir, "bin", "kafka-console-consumer.sh"),
		"--bootstrap-server", kafkaURI,
		"--topic", topic,
		"--from-beginning",
		"--max-messages", fmt.Sprintf("%d", count),
	)
	// Send stderr to our stderr so the user can see any error messages.
	readCmd.Stderr = os.Stderr

	// Kill both processes before we exit.
	defer func() { runCmd.Process.Kill() }()  // nolint: errcheck
	defer func() { readCmd.Process.Kill() }() // nolint: errcheck

	// Run the command, read the messages and wait for a timeout in parallel.
	go func() {
		// Read all of stdout.
		defer func() {
			if err := recover(); err != nil {
				if errv, ok := err.(error); ok {
					done <- result{nil, errv}
				} else {
					panic(err)
				}
			}
		}()
		data, err := readCmd.Output()
		checkQueryAPI()
		done <- result{data, err}
	}()
	go func() {
		err := runCmd.Run()
		done <- result{nil, err}
	}()
	go func() {
		time.Sleep(timeout)
		done <- result{nil, fmt.Errorf("Timeout reading %d messages from topic %q", count, topic)}
	}()

	// Poll the HTTP listener of the process waiting for it to be ready to receive requests.
	ready := make(chan struct{})
	go func() {
		delay := 10 * time.Millisecond
		for {
			time.Sleep(delay)
			if delay < 100*time.Millisecond {
				delay *= 2
			}
			resp, err := http.Get(readyURL)
			if err != nil {
				continue
			}
			if resp.StatusCode == 200 {
				break
			}
		}
		ready <- struct{}{}
	}()

	// Wait for the roomserver to be ready to receive input or for it to crash.
	select {
	case <-ready:
	case r := <-done:
		return nil, r.err
	}

	// Write the input now that the server is running.
	doInput()

	// Wait for one of the tasks to finsh.
	r := <-done

	if r.err != nil {
		return nil, r.err
	}

	// The kafka console consumer writes a newline character after each message.
	// So we split on newline characters
	lines := strings.Split(string(r.data), "\n")
	if len(lines) > 0 {
		// Remove the blank line at the end of the data.
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func writeToRoomServer(input []string, roomserverURL string) error {
	var request api.InputRoomEventsRequest
	var response api.InputRoomEventsResponse
	var err error
	request.InputRoomEvents = make([]api.InputRoomEvent, len(input))
	for i := range input {
		if err = json.Unmarshal([]byte(input[i]), &request.InputRoomEvents[i]); err != nil {
			return err
		}
	}
	x, err := inthttp.NewRoomserverClient(roomserverURL, &http.Client{Timeout: timeoutHTTP}, nil)
	if err != nil {
		return err
	}
	return x.InputRoomEvents(context.Background(), &request, &response)
}

// testRoomserver is used to run integration tests against a single roomserver.
// It creates new kafka topics for the input and output of the roomserver.
// It writes the input messages to the input kafka topic, formatting each message
// as canonical JSON so that it fits on a single line.
// It then runs the roomserver and waits for a number of messages to be written
// to the output topic.
// Once those messages have been written it runs the checkQueries function passing
// a api.RoomserverQueryAPI client. The caller can use this function to check the
// behaviour of the query API.
func testRoomserver(input []string, wantOutput []string, checkQueries func(api.RoomserverInternalAPI)) {
	dir, err := ioutil.TempDir("", "room-server-test")
	if err != nil {
		panic(err)
	}

	cfg, _, err := test.MakeConfig(dir, kafkaURI, testDatabase, "localhost", 10000)
	if err != nil {
		panic(err)
	}
	if err = test.WriteConfig(cfg, dir); err != nil {
		panic(err)
	}

	outputTopic := cfg.Global.Kafka.TopicFor(config.TopicOutputRoomEvent)

	err = exe.DeleteTopic(outputTopic)
	if err != nil {
		panic(err)
	}

	if err = exe.CreateTopic(outputTopic); err != nil {
		panic(err)
	}

	if err = createDatabase(testDatabaseName); err != nil {
		panic(err)
	}

	cache, err := caching.NewInMemoryLRUCache(false)
	if err != nil {
		panic(err)
	}

	doInput := func() {
		fmt.Printf("Roomserver is ready to receive input, sending %d events\n", len(input))
		if err = writeToRoomServer(input, cfg.RoomServerURL()); err != nil {
			panic(err)
		}
	}

	cmd := exec.Command(filepath.Join(filepath.Dir(os.Args[0]), "dendrite-room-server"))

	// Append the roomserver config to the existing environment.
	// We append to the environment rather than replacing so that any additional
	// postgres and golang environment variables such as PGHOST are passed to
	// the roomserver process.
	cmd.Stderr = os.Stderr
	cmd.Args = []string{"dendrite-room-server", "--config", filepath.Join(dir, test.ConfigFile)}

	gotOutput, err := runAndReadFromTopic(cmd, cfg.RoomServerURL()+"/metrics", doInput, outputTopic, len(wantOutput), func() {
		queryAPI, _ := inthttp.NewRoomserverClient("http://"+string(cfg.RoomServer.InternalAPI.Connect), &http.Client{Timeout: timeoutHTTP}, cache)
		checkQueries(queryAPI)
	})
	if err != nil {
		panic(err)
	}

	if len(wantOutput) != len(gotOutput) {
		panic(fmt.Errorf("Wanted %d lines of output got %d lines", len(wantOutput), len(gotOutput)))
	}

	for i := range wantOutput {
		if !equalJSON(wantOutput[i], gotOutput[i]) {
			panic(fmt.Errorf("Wanted %q at index %d got %q", wantOutput[i], i, gotOutput[i]))
		}
	}
}

func equalJSON(a, b string) bool {
	canonicalA, err := gomatrixserverlib.CanonicalJSON([]byte(a))
	if err != nil {
		panic(err)
	}
	canonicalB, err := gomatrixserverlib.CanonicalJSON([]byte(b))
	if err != nil {
		panic(err)
	}
	return string(canonicalA) == string(canonicalB)
}

func main() {
	fmt.Println("==TESTING==", os.Args[0])

	input := []string{
		`{
			"auth_event_ids": [],
			"kind": 1,
			"event": {
				"origin": "matrix.org",
				"signatures": {
					"matrix.org": {
						"ed25519:auto": "3kXGwNtdj+zqEXlI8PWLiB76xtrQ7SxcvPuXAEVCTo+QPoBoUvLi1RkHs6O5mDz7UzIowK5bi1seAN4vOh0OBA"
					}
				},
				"origin_server_ts": 1463671337837,
				"sender": "@richvdh:matrix.org",
				"event_id": "$1463671337126266wrSBX:matrix.org",
				"prev_events": [],
				"state_key": "",
				"content": {"creator": "@richvdh:matrix.org"},
				"depth": 1,
				"prev_state": [],
				"room_id": "!HCXfdvrfksxuYnIFiJ:matrix.org",
				"auth_events": [],
				"hashes": {"sha256": "Q05VLC8nztN2tguy+KnHxxhitI95wK9NelnsDaXRqeo"},
				"type": "m.room.create"}
		}`, `{
			"auth_event_ids": ["$1463671337126266wrSBX:matrix.org"],
			"kind": 2,
			"state_event_ids": ["$1463671337126266wrSBX:matrix.org"],
			"event": {
				"origin": "matrix.org",
				"signatures": {
					"matrix.org": {
						"ed25519:auto": "a2b3xXYVPPFeG1sHCU3hmZnAaKqZFgzGZozijRGblG5Y//ewRPAn1A2mCrI2UM5I+0zqr70cNpHgF8bmNFu4BA"
					}
				},
				"origin_server_ts": 1463671339844,
				"sender": "@richvdh:matrix.org",
				"event_id": "$1463671339126270PnVwC:matrix.org",
				"prev_events": [[
					"$1463671337126266wrSBX:matrix.org", {"sha256": "h/VS07u8KlMwT3Ee8JhpkC7sa1WUs0Srgs+l3iBv6c0"}
				]],
				"membership": "join",
				"state_key": "@richvdh:matrix.org",
				"content": {
					"membership": "join",
					"avatar_url": "mxc://matrix.org/ZafPzsxMJtLaSaJXloBEKiws",
					"displayname": "richvdh"
				},
				"depth": 2,
				"prev_state": [],
				"room_id": "!HCXfdvrfksxuYnIFiJ:matrix.org",
				"auth_events": [[
					"$1463671337126266wrSBX:matrix.org", {"sha256": "h/VS07u8KlMwT3Ee8JhpkC7sa1WUs0Srgs+l3iBv6c0"}
				]],
				"hashes": {"sha256": "t9t3sZV1Eu0P9Jyrs7pge6UTa1zuTbRdVxeUHnrQVH0"},
				"type": "m.room.member"},
			"has_state": true
		}`,
	}

	want := []string{
		`{"type":"new_room_event","new_room_event":{
			"event":{
				"auth_events":[[
					"$1463671337126266wrSBX:matrix.org",{"sha256":"h/VS07u8KlMwT3Ee8JhpkC7sa1WUs0Srgs+l3iBv6c0"}
				]],
				"content":{
					"avatar_url":"mxc://matrix.org/ZafPzsxMJtLaSaJXloBEKiws",
					"displayname":"richvdh",
					"membership":"join"
				},
				"depth": 2,
				"event_id": "$1463671339126270PnVwC:matrix.org",
				"hashes": {"sha256":"t9t3sZV1Eu0P9Jyrs7pge6UTa1zuTbRdVxeUHnrQVH0"},
				"membership": "join",
				"origin": "matrix.org",
				"origin_server_ts": 1463671339844,
				"prev_events": [[
					"$1463671337126266wrSBX:matrix.org",{"sha256":"h/VS07u8KlMwT3Ee8JhpkC7sa1WUs0Srgs+l3iBv6c0"}
				]],
				"prev_state":[],
				"room_id":"!HCXfdvrfksxuYnIFiJ:matrix.org",
				"sender":"@richvdh:matrix.org",
				"signatures":{
					"matrix.org":{
						"ed25519:auto":"a2b3xXYVPPFeG1sHCU3hmZnAaKqZFgzGZozijRGblG5Y//ewRPAn1A2mCrI2UM5I+0zqr70cNpHgF8bmNFu4BA"
					}
				},
				"state_key":"@richvdh:matrix.org",
				"type":"m.room.member"
			},
			"state_before_removes_event_ids":["$1463671339126270PnVwC:matrix.org"],
			"state_before_adds_event_ids":null,
			"latest_event_ids":["$1463671339126270PnVwC:matrix.org"],
			"adds_state_event_ids":["$1463671337126266wrSBX:matrix.org", "$1463671339126270PnVwC:matrix.org"],
			"removes_state_event_ids":null,
			"last_sent_event_id":"",
			"send_as_server":"",
			"transaction_id": null
		}}`,
	}

	testRoomserver(input, want, func(q api.RoomserverInternalAPI) {
		var response api.QueryLatestEventsAndStateResponse
		if err := q.QueryLatestEventsAndState(
			context.Background(),
			&api.QueryLatestEventsAndStateRequest{
				RoomID: "!HCXfdvrfksxuYnIFiJ:matrix.org",
				StateToFetch: []gomatrixserverlib.StateKeyTuple{
					{EventType: "m.room.member", StateKey: "@richvdh:matrix.org"},
				},
			},
			&response,
		); err != nil {
			panic(err)
		}
		if !response.RoomExists {
			panic(fmt.Errorf(`Wanted room "!HCXfdvrfksxuYnIFiJ:matrix.org" to exist`))
		}
		if len(response.LatestEvents) != 1 || response.LatestEvents[0].EventID != "$1463671339126270PnVwC:matrix.org" {
			panic(fmt.Errorf(`Wanted "$1463671339126270PnVwC:matrix.org" to be the latest event got %#v`, response.LatestEvents))
		}
		if len(response.StateEvents) != 1 || response.StateEvents[0].EventID() != "$1463671339126270PnVwC:matrix.org" {
			panic(fmt.Errorf(`Wanted "$1463671339126270PnVwC:matrix.org" to be the state event got %#v`, response.StateEvents))
		}
	})

	fmt.Println("==PASSED==", os.Args[0])
}
