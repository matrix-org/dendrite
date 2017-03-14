// Generate a list of matrix room events for load testing.
// Writes the events to stdout by default.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
	"os"
	"strconv"
	"time"
)

var (
	// The name of the matrix server to generate events for.
	serverName = defaulting(os.Getenv("SERVER_NAME"), "localhost")
	// The ID of the key used to sign the events.
	keyID = defaulting(os.Getenv("KEY_ID"), "ed25519:auto")
	// Base64 encoded private key to sign the events with.
	privateKeyString = defaulting(os.Getenv("PRIVATE_KEY"), defaultKey)
	// The room ID to generate events in.
	roomID = defaulting(os.Getenv("ROOM_ID"), "!roomid:"+serverName)
	// The user ID to use for as the event sender.
	userID = defaulting(os.Getenv("USER_ID"), "@userid:"+serverName)
	// The number of m.room.messsage events to generate.
	messageCount = defaulting(os.Getenv("MESSAGE_COUNT"), "10")
	// The output format to use for the messages.
	//	INPUT -> api.InputRoomEvent
	//  RAW   -> gomatrixserverlib.Event
	format = defaulting(os.Getenv("FORMAT"), "INPUT")
)

func defaulting(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// By default we use a private key of 0.
const defaultKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var privateKey ed25519.PrivateKey
var emptyString = ""
var now time.Time
var b gomatrixserverlib.EventBuilder
var eventID int

func main() {
	// Decode the ed25519 private key.
	privateKeyBytes, err := base64.RawStdEncoding.DecodeString(privateKeyString)
	if err != nil {
		panic(err)
	}
	privateKey = ed25519.PrivateKey(privateKeyBytes)

	count, err := strconv.Atoi(messageCount)
	if err != nil {
		panic(err)
	}

	// Build a m.room.create event.
	b.Sender = userID
	b.RoomID = roomID
	b.Type = "m.room.create"
	b.StateKey = &emptyString
	b.SetContent(map[string]string{"creator": userID})
	create := build()

	// Build a m.room.member event.
	b.Type = "m.room.member"
	b.StateKey = &userID
	b.SetContent(map[string]string{"membership": "join"})
	b.AuthEvents = []gomatrixserverlib.EventReference{create}
	member := build()

	// Build a number of m.room.message events.
	b.Type = "m.room.message"
	b.StateKey = nil
	b.SetContent(map[string]string{"body": "Test Message"})
	b.AuthEvents = []gomatrixserverlib.EventReference{create, member}
	for i := 0; i < count; i++ {
		build()
	}
}

// Build an event and write the event to the output.
func build() gomatrixserverlib.EventReference {
	eventID++
	id := fmt.Sprintf("$%d:%s", eventID, serverName)
	now = time.Unix(0, 0)
	event, err := b.Build(id, now, serverName, keyID, privateKey)
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
	if format == "INPUT" {
		var ire api.InputRoomEvent
		ire.Kind = api.KindNew
		ire.Event = event.JSON()
		authEventIDs := []string{}
		for _, ref := range b.AuthEvents {
			authEventIDs = append(authEventIDs, ref.EventID)
		}
		ire.AuthEventIDs = authEventIDs
		if err := encoder.Encode(ire); err != nil {
			panic(err)
		}
	} else if format == "RAW" {
		if err := encoder.Encode(event); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Errorf("Format %q is not valid, must be %q or %q", format, "RAW", "INPUT"))
	}
}
