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

// Generate a list of matrix room events for load testing.
// Writes the events to stdout by default.
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
)

const usage = `Usage: %s

Generate a list of matrix room events for load testing.
Writes the events to stdout separated by new lines

Arguments:

`

var (
	serverName       = flag.String("server-name", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_SERVERNAME", "localhost"), "The name of the matrix server to generate events for")
	keyID            = flag.String("key-id", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_KEYID", "ed25519:auto"), "The ID of the key used to sign the events")
	privateKeyString = flag.String("private-key", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_PRIVATE_KEY", defaultKey), "Base64 encoded private key to sign events with")
	roomID           = flag.String("room-id", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_ROOMID", "!roomid:$SERVER_NAME"), "The room ID to generate events in")
	userID           = flag.String("user-id", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_USERID", "@userid:$SERVER_NAME"), "The user ID to use as the event sender")
	messageCount     = flag.Int("message-count", 10, "The number of m.room.messsage events to generate")
	format           = flag.String("Format", basecomponent.EnvParse("DENDRITE_CREATE_ROOM_EVENTS_FORMAT", "InputRoomEvent"), "The output format to use for the messages: InputRoomEvent or Event")
)

// By default we use a private key of 0.
const defaultKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var privateKey ed25519.PrivateKey
var emptyString = ""
var now time.Time
var b gomatrixserverlib.EventBuilder
var eventID int

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	*userID = strings.Replace(*userID, "$SERVER_NAME", *serverName, 1)
	*roomID = strings.Replace(*roomID, "$SERVER_NAME", *serverName, 1)

	// Decode the ed25519 private key.
	privateKeyBytes, err := base64.RawStdEncoding.DecodeString(*privateKeyString)
	if err != nil {
		panic(err)
	}
	privateKey = ed25519.PrivateKey(privateKeyBytes)

	// Build a m.room.create event.
	b.Sender = *userID
	b.RoomID = *roomID
	b.Type = "m.room.create"
	b.StateKey = &emptyString
	b.SetContent(map[string]string{"creator": *userID}) // nolint: errcheck
	create := buildAndOutput()

	// Build a m.room.member event.
	b.Type = "m.room.member"
	b.StateKey = userID
	b.SetContent(map[string]string{"membership": gomatrixserverlib.Join}) // nolint: errcheck
	b.AuthEvents = []gomatrixserverlib.EventReference{create}
	member := buildAndOutput()

	// Build a number of m.room.message events.
	b.Type = "m.room.message"
	b.StateKey = nil
	b.SetContent(map[string]string{"body": "Test Message"}) // nolint: errcheck
	b.AuthEvents = []gomatrixserverlib.EventReference{create, member}
	for i := 0; i < *messageCount; i++ {
		buildAndOutput()
	}
}

// Build an event and write the event to the output.
func buildAndOutput() gomatrixserverlib.EventReference {
	eventID++
	id := fmt.Sprintf("$%d:%s", eventID, *serverName)
	now = time.Unix(0, 0)
	name := gomatrixserverlib.ServerName(*serverName)
	key := gomatrixserverlib.KeyID(*keyID)

	event, err := b.Build(id, now, name, key, privateKey)
	if err != nil {
		panic(err)
	}
	writeEvent(event)
	reference := event.EventReference()
	b.PrevEvents = []gomatrixserverlib.EventReference{reference}
	b.Depth++
	return reference
}

// Write an event to the output.
func writeEvent(event gomatrixserverlib.Event) {
	encoder := json.NewEncoder(os.Stdout)
	if *format == "InputRoomEvent" {
		var ire api.InputRoomEvent
		ire.Kind = api.KindNew
		ire.Event = event
		authEventIDs := []string{}
		for _, ref := range b.AuthEvents {
			authEventIDs = append(authEventIDs, ref.EventID)
		}
		ire.AuthEventIDs = authEventIDs
		if err := encoder.Encode(ire); err != nil {
			panic(err)
		}
	} else if *format == "Event" {
		if err := encoder.Encode(event); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("Format %q is not valid, must be %q or %q", *format, "InputRoomEvent", "Event"))
	}
}
