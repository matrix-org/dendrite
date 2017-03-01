package main

import (
	"fmt"
	"github.com/matrix-org/gomatrixserverlib"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	kafkaDir         = defaulting(os.Getenv("KAFKA_DIR"), "kafka")
	zookeeperURI     = defaulting(os.Getenv("ZOOKEEPER_URI"), "localhost:2181")
	kafkaURI         = defaulting(os.Getenv("KAFKA_URIS"), "localhost:9092")
	timeoutString    = defaulting(os.Getenv("TIMEOUT"), "10s")
	postgresDatabase = defaulting(os.Getenv("POSTGRES_DATABASE"), "postgres")
	testDatabaseName = defaulting(os.Getenv("DATABASE_NAME"), "roomserver_test")
	testDatabase     = defaulting(os.Getenv("DATABASE"), fmt.Sprintf("dbname=%s binary_parameters=yes", testDatabaseName))
)

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

func createDatabase(database string) error {
	cmd := exec.Command("psql", postgresDatabase)
	cmd.Stdin = strings.NewReader(
		fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s;", database, database),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createTopic(topic string) error {
	cmd := exec.Command(
		filepath.Join(kafkaDir, "bin", "kafka-topics.sh"),
		"--create",
		"--zookeeper", zookeeperURI,
		"--replication-factor", "1",
		"--partitions", "1",
		"--topic", topic,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeToTopic(topic string, data []string) error {
	cmd := exec.Command(
		filepath.Join(kafkaDir, "bin", "kafka-console-producer.sh"),
		"--broker-list", kafkaURI,
		"--topic", topic,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(strings.Join(data, "\n"))
	return cmd.Run()
}

func runAndReadFromTopic(runCmd *exec.Cmd, topic string, count int) ([]string, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result)
	readCmd := exec.Command(
		filepath.Join(kafkaDir, "bin", "kafka-console-consumer.sh"),
		"--bootstrap-server", kafkaURI,
		"--topic", topic,
		"--from-beginning",
		"--max-messages", fmt.Sprintf("%d", count),
	)
	readCmd.Stderr = os.Stderr
	go func() {
		data, err := readCmd.Output()
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
	r := <-done

	runCmd.Process.Kill()
	readCmd.Process.Kill()
	lines := strings.Split(string(r.data), "\n")
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, r.err
}

func deleteTopic(topic string) error {
	cmd := exec.Command(
		filepath.Join(kafkaDir, "bin", "kafka-topics.sh"),
		"--delete",
		"--if-exists",
		"--zookeeper", zookeeperURI,
		"--topic", topic,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	return cmd.Run()
}

func testRoomServer(input []string, wantOutput []string) {
	const (
		inputTopic  = "roomserverInput"
		outputTopic = "roomserverOutput"
	)
	deleteTopic(inputTopic)
	if err := createTopic(inputTopic); err != nil {
		panic(err)
	}
	deleteTopic(outputTopic)
	if err := createTopic(outputTopic); err != nil {
		panic(err)
	}

	if err := writeToTopic(inputTopic, canonicalJSONInput(input)); err != nil {
		panic(err)
	}

	if err := createDatabase(testDatabaseName); err != nil {
		panic(err)
	}

	cmd := exec.Command(filepath.Join(filepath.Dir(os.Args[0]), "roomserver"))

	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("DATABASE=%s", testDatabase),
		fmt.Sprintf("KAFKA_URIS=%s", kafkaURI),
		fmt.Sprintf("TOPIC_INPUT_ROOM_EVENT=%s", inputTopic),
		fmt.Sprintf("TOPIC_OUTPUT_ROOM_EVENT=%s", outputTopic),
	)
	cmd.Stderr = os.Stderr

	gotOutput, err := runAndReadFromTopic(cmd, outputTopic, 1)
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
			"AuthEventIDs": [],
			"Kind": 1,
			"Event": {
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
			"AuthEventIDs": ["$1463671337126266wrSBX:matrix.org"],
			"Kind": 2,
			"StateEventIDs": ["$1463671337126266wrSBX:matrix.org"],
			"Event": {
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
			"HasState": true
		}`,
	}

	want := []string{
		`{
			"Event":{
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
			"VisibilityEventIDs":null,
			"LatestEventIDs":["$1463671339126270PnVwC:matrix.org"],
			"AddsStateEventIDs":null,
			"RemovesStateEventIDs":null,
			"LastSentEventID":""
		}`,
	}

	testRoomServer(input, want)

	fmt.Println("==PASSED==", os.Args[0])
}
