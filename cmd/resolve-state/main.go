package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
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
//   e.g. ./resolve-state -room_id '!abc:localhost'

var roomVersion = flag.String("roomversion", "5", "the room version to parse events as")
var filterType = flag.String("filtertype", "", "the event types to filter on")
var difference = flag.Bool("difference", false, "whether to calculate the difference between snapshots")
var roomID = flag.String("room_id", "", "roomID to get the state for, using this flag ignores any passed snapshot NIDs and calculates the resolved state using ALL state snapshots")
var fixState = flag.Bool("fix", false, "attempt to fix the room state")

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
	if *roomID != "" {
		roomInfo, err = roomserverDB.RoomInfo(ctx, *roomID)
		if err != nil {
			panic(err)
		}
		if roomInfo == nil {
			panic("no room found")
		}

		snapshotNIDs, err = roomserverDB.GetAllStateSnapshots(ctx, roomInfo.RoomNID)
		if err != nil {
			panic(err)
		}

	}

	fmt.Println("Room version", roomInfo.RoomVersion)

	stateres := state.NewStateResolution(roomserverDB, roomInfo, rsAPI)

	fmt.Println("Fetching", len(snapshotNIDs), "snapshot NIDs")

	if *difference {
		showDifference(ctx, snapshotNIDs, stateres, roomserverDB, roomInfo)
		return
	}

	var stateEntries []types.StateEntry

	for i, snapshotNID := range snapshotNIDs {
		fmt.Printf("\r \a %d of %d", i, len(snapshotNIDs))
		var entries []types.StateEntry
		entries, err = stateres.LoadStateAtSnapshot(ctx, snapshotNID)
		if err != nil {
			panic(err)
		}
		stateEntries = append(stateEntries, entries...)
	}
	fmt.Println()

	eventNIDMap := map[types.EventNID]types.StateEntry{}
	for _, entry := range stateEntries {
		eventNIDMap[entry.EventNID] = entry
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
	eventIDNIDMap := make(map[string]types.EventNID)
	for i := range eventEntries {
		eventIDNIDMap[eventEntries[i].EventID()] = eventEntries[i].EventNID
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
	resolvedRoomID := ""
	for i := range authEventEntries {
		authEvents[i] = authEventEntries[i].PDU
		if authEvents[i].RoomID().String() != "" {
			resolvedRoomID = authEvents[i].RoomID().String()
		}
	}

	// Get the roomNID
	roomInfo, err = roomserverDB.RoomInfo(ctx, resolvedRoomID)
	if err != nil {
		panic(err)
	}

	fmt.Println("Resolving state")
	stateResStart := time.Now()
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

	fmt.Printf("Resolved state contains %d events (resolution took %s)\n", len(resolved), time.Since(stateResStart))
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

	if !*fixState {
		return
	}

	fmt.Println()
	fmt.Printf("\t\t !!! WARNING !!!\n")
	fmt.Println("Attempting to fix the state of a room can make things even worse.")
	fmt.Println("For the best result, please shut down Dendrite to avoid concurrent database changes.")
	fmt.Println("If you have missing state events (e.g. users not in a room, missing power levels")
	fmt.Println("make sure they would be added by checking the resolved state events above first (or by running without -fix).")
	fmt.Println("If you are sure everything looks fine, press Return, if not, press CTRL+c.")
	fmt.Scanln()

	fmt.Println("Attempting to fix state")

	initialSnapshotNID := roomInfo.StateSnapshotNID()

	stateEntriesResolved := make([]types.StateEntry, len(resolved))
	for i := range resolved {
		eventNID := eventIDNIDMap[resolved[i].EventID()]
		stateEntriesResolved[i] = eventNIDMap[eventNID]
	}

	var succeeded bool
	roomUpdater, err := roomserverDB.GetRoomUpdater(ctx, roomInfo)
	if err != nil {
		panic(err)
	}
	defer sqlutil.EndTransactionWithCheck(roomUpdater, &succeeded, &err)

	latestEvents := make([]types.StateAtEventAndReference, 0, len(roomUpdater.LatestEvents()))
	for _, event := range roomUpdater.LatestEvents() {
		// SetLatestEvents only uses the EventNID, so populate that
		latestEvents = append(latestEvents, types.StateAtEventAndReference{
			StateAtEvent: types.StateAtEvent{
				StateEntry: types.StateEntry{
					EventNID: event.EventNID,
				},
			},
		})
	}

	var lastEventSent []types.Event
	lastEventSent, err = roomUpdater.EventsFromIDs(ctx, roomInfo, []string{roomUpdater.LastEventIDSent()})
	if err != nil {
		fmt.Printf("Error: %s", err)
		return
	}
	if len(lastEventSent) != 1 {
		fmt.Printf("Error: expected to get one event from the database but didn't, got %d", len(lastEventSent))
		return
	}

	var newSnapshotNID types.StateSnapshotNID
	newSnapshotNID, err = roomUpdater.AddState(ctx, roomInfo.RoomNID, nil, stateEntriesResolved)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return
	}

	if err = roomUpdater.SetLatestEvents(roomInfo.RoomNID, latestEvents, lastEventSent[0].EventNID, newSnapshotNID); err != nil {
		fmt.Printf("Error: %s", err)
		return
	}
	succeeded = true
	if err = roomUpdater.Commit(); err != nil {
		panic(err)
	}
	fmt.Printf("Successfully set new snapshot NID %d containing %d state events\n", newSnapshotNID, len(stateEntriesResolved))
	showDifference(ctx, []types.StateSnapshotNID{newSnapshotNID, initialSnapshotNID}, stateres, roomserverDB, roomInfo)

}

func showDifference(ctx context.Context, snapshotNIDs []types.StateSnapshotNID, stateres state.StateResolution, roomserverDB storage.Database, roomInfo *types.RoomInfo) {
	if len(snapshotNIDs) != 2 {
		panic("need exactly two state snapshot NIDs to calculate difference")
	}

	removed, added, err := stateres.DifferenceBetweeenStateSnapshots(ctx, snapshotNIDs[0], snapshotNIDs[1])
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
