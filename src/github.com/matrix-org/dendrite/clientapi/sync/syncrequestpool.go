package sync

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db    *storage.SyncServerDatabase
	conns map[string]chan interface{}
}

// OnIncomingSyncRequest is called when a client makes a /sync request
func (srp *RequestPool) OnIncomingSyncRequest(req *http.Request) util.JSONResponse {
	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// OnNewEvent is called when a new event is received from the room server
func (srp *RequestPool) OnNewEvent(ev *gomatrixserverlib.Event) {

}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase) RequestPool {
	return RequestPool{db, make(map[string]chan interface{})}
}
