package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// This is a utility for inspecting state snapshots and running state resolution
// against real snapshots in an actual database.
// It takes one or more state snapshot NIDs as arguments, along with a room version
// to use for unmarshalling events, and will produce resolved output.
//
// Usage: ./resolve-state --roomversion=version snapshot [snapshot ...]
//   e.g. ./resolve-state --roomversion=5 1254 1235 1282

var roomVersion = flag.String("roomversion", "5", "the room version to parse events as")

// nolint:gocyclo
func main() {
	ctx := context.Background()
	cfg := setup.ParseFlags(true)
	args := os.Args[1:]

	fmt.Println("Room version", *roomVersion)

	snapshotNIDs := []types.StateSnapshotNID{}
	for _, arg := range args {
		if i, err := strconv.Atoi(arg); err == nil {
			snapshotNIDs = append(snapshotNIDs, types.StateSnapshotNID(i))
		}
	}

	fmt.Println("Fetching", len(snapshotNIDs), "snapshot NIDs")

	cache, err := caching.NewInMemoryLRUCache(true)
	if err != nil {
		panic(err)
	}

	roomserverDB, err := storage.Open(&cfg.RoomServer.Database, cache)
	if err != nil {
		panic(err)
	}

	blockNIDs, err := roomserverDB.StateBlockNIDs(ctx, snapshotNIDs)
	if err != nil {
		panic(err)
	}

	var stateEntries []types.StateEntryList
	for _, list := range blockNIDs {
		entries, err2 := roomserverDB.StateEntries(ctx, list.StateBlockNIDs)
		if err2 != nil {
			panic(err2)
		}
		stateEntries = append(stateEntries, entries...)
	}

	var eventNIDs []types.EventNID
	for _, entry := range stateEntries {
		for _, e := range entry.StateEntries {
			eventNIDs = append(eventNIDs, e.EventNID)
		}
	}

	fmt.Println("Fetching", len(eventNIDs), "state events")
	eventEntries, err := roomserverDB.Events(ctx, eventNIDs)
	if err != nil {
		panic(err)
	}

	authEventIDMap := make(map[string]struct{})
	eventPtrs := make([]*gomatrixserverlib.Event, len(eventEntries))
	for i := range eventEntries {
		eventPtrs[i] = &eventEntries[i].Event
		for _, authEventID := range eventEntries[i].AuthEventIDs() {
			authEventIDMap[authEventID] = struct{}{}
		}
	}

	authEventIDs := make([]string, 0, len(authEventIDMap))
	for authEventID := range authEventIDMap {
		authEventIDs = append(authEventIDs, authEventID)
	}

	fmt.Println("Fetching", len(authEventIDs), "auth events")
	authEventEntries, err := roomserverDB.EventsFromIDs(ctx, authEventIDs)
	if err != nil {
		panic(err)
	}

	authEventPtrs := make([]*gomatrixserverlib.Event, len(authEventEntries))
	for i := range authEventEntries {
		authEventPtrs[i] = &authEventEntries[i].Event
	}

	events := make([]gomatrixserverlib.Event, len(eventEntries))
	authEvents := make([]gomatrixserverlib.Event, len(authEventEntries))
	for i, ptr := range eventPtrs {
		events[i] = *ptr
	}
	for i, ptr := range authEventPtrs {
		authEvents[i] = *ptr
	}

	fmt.Println("Resolving state")
	resolved, err := state.ResolveConflictsAdhoc(
		gomatrixserverlib.RoomVersion(*roomVersion),
		events,
		authEvents,
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Resolved state contains", len(resolved), "events")
	for _, event := range resolved {
		fmt.Println()
		fmt.Printf("* %s %s %q\n", event.EventID(), event.Type(), *event.StateKey())
		fmt.Printf("  %s\n", string(event.Content()))
	}
}
