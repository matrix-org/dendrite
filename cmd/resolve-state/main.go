package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/state"
	"github.com/element-hq/dendrite/roomserver/storage"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// This is a utility for inspecting state snapshots and running state resolution
// against real snapshots in an actual database.
// It takes one or more state snapshot NIDs as arguments, along with a room version
// to use for unmarshalling events, and will produce resolved output.
//
// Usage: ./resolve-state --roomversion=version snapshot [snapshot ...]
//   e.g. ./resolve-state --roomversion=5 1254 1235 1282

var roomVersion = flag.String("roomversion", "5", "the room version to parse events as")
var filterType = flag.String("filtertype", "", "the event types to filter on")
var difference = flag.Bool("difference", false, "whether to calculate the difference between snapshots")

// dummyQuerier implements QuerySenderIDAPI. Does **NOT** do any "magic" for pseudoID rooms
// to avoid having to "start" a full roomserver API.
type dummyQuerier struct{}

func (d dummyQuerier) QuerySenderIDForUser(ctx context.Context, roomID spec.RoomID, userID spec.UserID) (*spec.SenderID, error) {
	s := spec.SenderIDFromUserID(userID)
	return &s, nil
}

func (d dummyQuerier) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return senderID.ToUserID(), nil
}

// nolint:gocyclo
func main() {
	ctx := context.Background()
	cfg := setup.ParseFlags(true)
	cfg.Logging = append(cfg.Logging[:0], config.LogrusHook{
		Type:  "std",
		Level: "error",
	})
	cfg.ClientAPI.RegistrationDisabled = true

	args := flag.Args()

	fmt.Println("Room version", *roomVersion)

	snapshotNIDs := []types.StateSnapshotNID{}
	for _, arg := range args {
		if i, err := strconv.Atoi(arg); err == nil {
			snapshotNIDs = append(snapshotNIDs, types.StateSnapshotNID(i))
		}
	}

	processCtx := process.NewProcessContext()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

	dbOpts := cfg.RoomServer.Database
	if dbOpts.ConnectionString == "" {
		dbOpts = cfg.Global.DatabaseOptions
	}

	fmt.Println("Opening database")
	roomserverDB, err := storage.Open(
		processCtx.Context(), cm, &dbOpts,
		caching.NewRistrettoCache(8*1024*1024, time.Minute*5, caching.DisableMetrics),
	)
	if err != nil {
		panic(err)
	}

	rsAPI := dummyQuerier{}

	roomInfo := &types.RoomInfo{
		RoomVersion: gomatrixserverlib.RoomVersion(*roomVersion),
	}
	stateres := state.NewStateResolution(roomserverDB, roomInfo, rsAPI)

	fmt.Println("Fetching", len(snapshotNIDs), "snapshot NIDs")

	if *difference {
		if len(snapshotNIDs) != 2 {
			panic("need exactly two state snapshot NIDs to calculate difference")
		}
		var removed, added []types.StateEntry
		removed, added, err = stateres.DifferenceBetweeenStateSnapshots(ctx, snapshotNIDs[0], snapshotNIDs[1])
		if err != nil {
			panic(err)
		}

		eventNIDMap := map[types.EventNID]struct{}{}
		for _, entry := range append(removed, added...) {
			eventNIDMap[entry.EventNID] = struct{}{}
		}

		eventNIDs := make([]types.EventNID, 0, len(eventNIDMap))
		for eventNID := range eventNIDMap {
			eventNIDs = append(eventNIDs, eventNID)
		}

		var eventEntries []types.Event
		eventEntries, err = roomserverDB.Events(ctx, roomInfo.RoomVersion, eventNIDs)
		if err != nil {
			panic(err)
		}

		events := make(map[types.EventNID]gomatrixserverlib.PDU, len(eventEntries))
		for _, entry := range eventEntries {
			events[entry.EventNID] = entry.PDU
		}

		if len(removed) > 0 {
			fmt.Println("Removed:")
			for _, r := range removed {
				event := events[r.EventNID]
				fmt.Println()
				fmt.Printf("* %s %s %q\n", event.EventID(), event.Type(), *event.StateKey())
				fmt.Printf("  %s\n", string(event.Content()))
			}
		}

		if len(removed) > 0 && len(added) > 0 {
			fmt.Println()
		}

		if len(added) > 0 {
			fmt.Println("Added:")
			for _, a := range added {
				event := events[a.EventNID]
				fmt.Println()
				fmt.Printf("* %s %s %q\n", event.EventID(), event.Type(), *event.StateKey())
				fmt.Printf("  %s\n", string(event.Content()))
			}
		}

		return
	}

	var stateEntries []types.StateEntry
	for _, snapshotNID := range snapshotNIDs {
		var entries []types.StateEntry
		entries, err = stateres.LoadStateAtSnapshot(ctx, snapshotNID)
		if err != nil {
			panic(err)
		}
		stateEntries = append(stateEntries, entries...)
	}

	eventNIDMap := map[types.EventNID]struct{}{}
	for _, entry := range stateEntries {
		eventNIDMap[entry.EventNID] = struct{}{}
	}

	eventNIDs := make([]types.EventNID, 0, len(eventNIDMap))
	for eventNID := range eventNIDMap {
		eventNIDs = append(eventNIDs, eventNID)
	}

	fmt.Println("Fetching", len(eventNIDMap), "state events")
	eventEntries, err := roomserverDB.Events(ctx, roomInfo.RoomVersion, eventNIDs)
	if err != nil {
		panic(err)
	}

	authEventIDMap := make(map[string]struct{})
	events := make([]gomatrixserverlib.PDU, len(eventEntries))
	for i := range eventEntries {
		events[i] = eventEntries[i].PDU
		for _, authEventID := range eventEntries[i].AuthEventIDs() {
			authEventIDMap[authEventID] = struct{}{}
		}
	}

	authEventIDs := make([]string, 0, len(authEventIDMap))
	for authEventID := range authEventIDMap {
		authEventIDs = append(authEventIDs, authEventID)
	}

	fmt.Println("Fetching", len(authEventIDs), "auth events")
	authEventEntries, err := roomserverDB.EventsFromIDs(ctx, roomInfo, authEventIDs)
	if err != nil {
		panic(err)
	}

	authEvents := make([]gomatrixserverlib.PDU, len(authEventEntries))
	for i := range authEventEntries {
		authEvents[i] = authEventEntries[i].PDU
	}

	// Get the roomNID
	roomInfo, err = roomserverDB.RoomInfo(ctx, authEvents[0].RoomID().String())
	if err != nil {
		panic(err)
	}

	fmt.Println("Resolving state")
	var resolved Events
	resolved, err = gomatrixserverlib.ResolveConflicts(
		gomatrixserverlib.RoomVersion(*roomVersion), events, authEvents, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		},
		func(eventID string) bool {
			isRejected, rejectedErr := roomserverDB.IsEventRejected(ctx, roomInfo.RoomNID, eventID)
			if rejectedErr != nil {
				return true
			}
			return isRejected
		},
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Resolved state contains", len(resolved), "events")
	sort.Sort(resolved)
	filteringEventType := *filterType
	count := 0
	for _, event := range resolved {
		if filteringEventType != "" && event.Type() != filteringEventType {
			continue
		}
		count++
		fmt.Println()
		fmt.Printf("* %s %s %q\n", event.EventID(), event.Type(), *event.StateKey())
		fmt.Printf("  %s\n", string(event.Content()))
	}

	fmt.Println()
	fmt.Println("Returned", count, "state events after filtering")
}

type Events []gomatrixserverlib.PDU

func (e Events) Len() int {
	return len(e)
}

func (e Events) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e Events) Less(i, j int) bool {
	typeDelta := strings.Compare(e[i].Type(), e[j].Type())
	if typeDelta < 0 {
		return true
	}
	if typeDelta > 0 {
		return false
	}
	stateKeyDelta := strings.Compare(*e[i].StateKey(), *e[j].StateKey())
	return stateKeyDelta < 0
}
