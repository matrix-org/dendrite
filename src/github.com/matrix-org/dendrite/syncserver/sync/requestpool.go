package sync

import (
	"net/http"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncserver/storage"
	"github.com/matrix-org/dendrite/syncserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db *storage.SyncServerDatabase
	// The latest sync stream position: guarded by 'cond'.
	currPos types.StreamPosition
	// A condition variable to notify all waiting goroutines of a new sync stream position
	cond *sync.Cond
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase) (*RequestPool, error) {
	pos, err := db.SyncStreamPosition()
	if err != nil {
		return nil, err
	}
	return &RequestPool{db, types.StreamPosition(pos), sync.NewCond(&sync.Mutex{})}, nil
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request) util.JSONResponse {
	// Extract values from request
	logger := util.GetLogger(req.Context())
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	syncReq, err := newSyncRequest(req, userID)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}
	logger.WithFields(log.Fields{
		"userID":  userID,
		"since":   syncReq.since,
		"timeout": syncReq.timeout,
	}).Info("Incoming /sync request")

	// Fork off 2 goroutines: one to do the work, and one to serve as a timeout.
	// Whichever returns first is the one we will serve back to the client.
	// TODO: Currently this means that cpu work is timed, which may not be what we want long term.
	timeoutChan := make(chan struct{})
	timer := time.AfterFunc(syncReq.timeout, func() {
		close(timeoutChan) // signal that the timeout has expired
	})

	done := make(chan util.JSONResponse)
	go func() {
		syncData, err := rp.currentSyncForUser(*syncReq)
		timer.Stop()
		var res util.JSONResponse
		if err != nil {
			res = httputil.LogThenError(req, err)
		} else {
			res = util.JSONResponse{
				Code: 200,
				JSON: syncData,
			}
		}
		done <- res
		close(done)
	}()

	select {
	case <-timeoutChan: // timeout fired
		return util.JSONResponse{
			Code: 200,
			JSON: types.NewResponse(syncReq.since),
		}
	case res := <-done: // received a response
		return res
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine, to avoid races between updates which could set the
// current position in the stream incorrectly.
func (rp *RequestPool) OnNewEvent(ev *gomatrixserverlib.Event, pos types.StreamPosition) {
	// update the current position in a guard and then notify all /sync streams
	rp.cond.L.Lock()
	rp.currPos = pos
	rp.cond.L.Unlock()

	rp.cond.Broadcast() // notify ALL waiting goroutines
}

func (rp *RequestPool) waitForEvents(req syncRequest) types.StreamPosition {
	// In a guard, check if the /sync request should block, and block it until we get a new position
	rp.cond.L.Lock()
	currentPos := rp.currPos
	for req.since == currentPos {
		// we need to wait for a new event.
		// TODO: This waits for ANY new event, we need to only wait for events which we care about.
		rp.cond.Wait() // atomically unlocks and blocks goroutine, then re-acquires lock on unblock
		currentPos = rp.currPos
	}
	rp.cond.L.Unlock()
	return currentPos
}

func (rp *RequestPool) currentSyncForUser(req syncRequest) (*types.Response, error) {
	if req.since == types.StreamPosition(0) {
		pos, data, err := rp.db.CompleteSync(req.userID, req.limit)
		if err != nil {
			return nil, err
		}
		res := types.NewResponse(pos)
		for roomID, d := range data {
			jr := types.NewJoinResponse()
			jr.Timeline.Events = gomatrixserverlib.ToClientEvents(d.RecentEvents, gomatrixserverlib.FormatSync)
			jr.Timeline.Limited = true
			jr.State.Events = gomatrixserverlib.ToClientEvents(d.State, gomatrixserverlib.FormatSync)
			res.Rooms.Join[roomID] = *jr
		}
		return res, nil
	}

	currentPos := rp.waitForEvents(req)

	// TODO: handle ignored users

	// TODO: Work out *which* rooms to return to the client
	// Based on https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821
	// 1) Get the CURRENT joined room list for this user
	// 2) Get membership list changes for this user between the provided stream position and now.
	// 3) For each room which has membership list changes:
	//   a) Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins).
	//      If it is, then we need to send the full room state down (and 'limited' is always true).
	//   b) Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
	//   c) Check if the user is CURRENTLY left/banned. If so, add room to 'archived' block. // Synapse has a TODO: How do we handle ban -> leave in same batch?
	// 4) Add joined rooms (joined room list)

	// Problem statement:
	// TODO: For each room being returned, work out *which* events to return to the client.
	// This involves calculating the room state at various points in the history of the room.
	// For example, imagine a room with the following 15 events (letters are state events (updated via '), numbers are timeline events):
	//
	// index     0  1  2  3  4  5  6  7  8   9  10  11  12  13    14     15   (1-based indexing as StreamPosition(0) represents no event)
	// timeline    [A, B, C, D, 1, 2, 3, D', 4, D'', 5, B', D''', D'''', 6]
	//
	// The current state of this room is: [A, B', C, D'''']
	//
	//
	// If this room was requested with ?since=9&limit=5 then 5 timeline events would be returned, the most recent ones:
	//    11 12  13    14     15
	//   [5, B', D''', D'''', 6]
	//
	// The state of the room at the START of the timeline can be represented in 2 ways:
	//  - The 'full_state' from index 0 : [A, B, C, D''] (aka the state between 0-11 exclusive)
	//  - A partial state from index 9  : [D'']          (aka the state between 9-11 exclusive)
	//
	// The /sync response returns the most recent timeline events and the state of the room at the start of the timeline, for each room.
	//
	// Servers advance state events (e.g from D' to D'') based on the state conflict resolution algorithm.
	// You might think that you could advance the current state by just updating the entry for the (event type, state_key) tuple
	// for each state event, but this state can diverge from the state calculated using the state conflict resolution algorithm.
	// For example, if there are two "simultaneous" updates to the same state key, that is two updates at the same depth in the
	// event graph, then the final result of the state conflict resolution algorithm might not match the order the events appear
	// in the timeline.
	//
	// The correct advancement for state events is represented by the add_state_ids and remove_state_ids that
	// are in OutputRoomEvents from the room server.

	// This version of dendrite uses very simple indexing to calculate room state at various points.
	// This is inefficient when a very old 'since' value is provided, or the 'full_state' is requested, as the state delta becomes
	// very large. This is mitigated slightly with indexes, but better data structures could be used.

	evs, err := rp.db.EventsInRange(req.since, currentPos)
	if err != nil {
		return nil, err
	}

	res := types.NewResponse(currentPos)
	// for now, dump everything as join timeline events
	for _, ev := range evs {
		roomData := res.Rooms.Join[ev.RoomID()]
		roomData.Timeline.Events = append(roomData.Timeline.Events, gomatrixserverlib.ToClientEvent(ev, gomatrixserverlib.FormatSync))
		res.Rooms.Join[ev.RoomID()] = roomData
	}

	// Make sure we send the next_batch as a string. We don't want to confuse clients by sending this
	// as an integer even though (at the moment) it is.
	res.NextBatch = currentPos.String()
	return res, nil
}
