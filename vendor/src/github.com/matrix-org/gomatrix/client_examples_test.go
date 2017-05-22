package gomatrix

import (
	"fmt"
	"net/http"
)

func Example_sync() {
	cli, _ := NewClient("https://matrix.org", "@example:matrix.org", "MDAefhiuwehfuiwe")
	cli.Store.SaveFilterID("@example:matrix.org", "2")                // Optional: if you know it already
	cli.Store.SaveNextBatch("@example:matrix.org", "111_222_333_444") // Optional: if you know it already
	syncer := cli.Syncer.(*DefaultSyncer)
	syncer.OnEventType("m.room.message", func(ev *Event) {
		fmt.Println("Message: ", ev)
	})

	// Blocking version
	if err := cli.Sync(); err != nil {
		fmt.Println("Sync() returned ", err)
	}

	// Non-blocking version
	go func() {
		for {
			if err := cli.Sync(); err != nil {
				fmt.Println("Sync() returned ", err)
			}
			// Optional: Wait a period of time before trying to sync again.
		}
	}()
}

func Example_customInterfaces() {
	// Custom interfaces must be set prior to calling functions on the client.
	cli, _ := NewClient("https://matrix.org", "@example:matrix.org", "MDAefhiuwehfuiwe")

	// anything which implements the Storer interface
	customStore := NewInMemoryStore()
	cli.Store = customStore

	// anything which implements the Syncer interface
	customSyncer := NewDefaultSyncer("@example:matrix.org", customStore)
	cli.Syncer = customSyncer

	// any http.Client
	cli.Client = http.DefaultClient

	// Once you call a function, you can't safely change the interfaces.
	cli.SendText("!foo:bar", "Down the rabbit hole")
}

func ExampleClient_BuildURLWithQuery() {
	cli, _ := NewClient("https://matrix.org", "@example:matrix.org", "abcdef123456")
	out := cli.BuildURLWithQuery([]string{"sync"}, map[string]string{
		"filter_id": "5",
	})
	fmt.Println(out)
	// Output: https://matrix.org/_matrix/client/r0/sync?access_token=abcdef123456&filter_id=5
}

func ExampleClient_BuildURL() {
	userID := "@example:matrix.org"
	cli, _ := NewClient("https://matrix.org", userID, "abcdef123456")
	out := cli.BuildURL("user", userID, "filter")
	fmt.Println(out)
	// Output: https://matrix.org/_matrix/client/r0/user/@example:matrix.org/filter?access_token=abcdef123456
}

func ExampleClient_BuildBaseURL() {
	userID := "@example:matrix.org"
	cli, _ := NewClient("https://matrix.org", userID, "abcdef123456")
	out := cli.BuildBaseURL("_matrix", "client", "r0", "directory", "room", "#matrix:matrix.org")
	fmt.Println(out)
	// Output: https://matrix.org/_matrix/client/r0/directory/room/%23matrix:matrix.org?access_token=abcdef123456
}

// Retrieve the content of a m.room.name state event.
func ExampleClient_StateEvent() {
	content := struct {
		Name string `json:"name"`
	}{}
	cli, _ := NewClient("https://matrix.org", "@example:matrix.org", "abcdef123456")
	if err := cli.StateEvent("!foo:bar", "m.room.name", "", &content); err != nil {
		panic(err)
	}
}

// Join a room by ID.
func ExampleClient_JoinRoom_id() {
	cli, _ := NewClient("http://localhost:8008", "@example:localhost", "abcdef123456")
	if _, err := cli.JoinRoom("!uOILRrqxnsYgQdUzar:localhost", "", nil); err != nil {
		panic(err)
	}
}

// Join a room by alias.
func ExampleClient_JoinRoom_alias() {
	cli, _ := NewClient("http://localhost:8008", "@example:localhost", "abcdef123456")
	if resp, err := cli.JoinRoom("#test:localhost", "", nil); err != nil {
		panic(err)
	} else {
		// Use room ID for something.
		_ = resp.RoomID
	}
}

// Login to a local homeserver and set the user ID and access token on success.
func ExampleClient_Login() {
	cli, _ := NewClient("http://localhost:8008", "", "")
	resp, err := cli.Login(&ReqLogin{
		Type:     "m.login.password",
		User:     "alice",
		Password: "wonderland",
	})
	if err != nil {
		panic(err)
	}
	cli.SetCredentials(resp.UserID, resp.AccessToken)
}
