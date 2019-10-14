package routing

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"net/http"
)

// SendToDevice this is a function for calling process of send-to-device messages those bypassed DAG
func SendToDevice(
	req *http.Request,
	sender string,
	syncDB *storage.SyncServerDatabase,
	deviceDB *devices.Database,
	eventType, txnID string,
	notifier *sync.Notifier,
) util.JSONResponse {
	ctx := req.Context()
	stdRq := types.StdRequest{}
	httputil.UnmarshalJSONRequest(req, &stdRq)
	for uid, deviceMap := range stdRq.Sender {

		// federation consideration todo:
		// if uid is remote domain a fed process should go
		if false {
			// federation process
			return util.JSONResponse{}
		}

		// uid is local domain
		for device, cont := range deviceMap {
			jsonBuffer, err := json.Marshal(cont)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: struct{}{},
				}
			}
			ev := types.StdHolder{
				Sender:   sender,
				Event:    jsonBuffer,
				EventTyp: eventType,
			}
			var pos int64

			// wildcard all devices
			if device == "*" {
				var deviceCollection []authtypes.Device
				var localpart string
				localpart, _, _ = gomatrixserverlib.SplitID('@', uid)
				deviceCollection, err = deviceDB.GetDevicesByLocalpart(ctx, localpart)
				for _, val := range deviceCollection {
					pos, err = syncDB.InsertStdMessage(ctx, ev, txnID, uid, val.ID)
					notifier.OnNewEvent(nil, uid, types.StreamPosition(pos))
				}
				if err != nil {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: struct{}{},
					}
				}
				return util.JSONResponse{
					Code: http.StatusOK,
					JSON: struct{}{},
				}
			}
			pos, err = syncDB.InsertStdMessage(ctx, ev, txnID, uid, device)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: struct{}{},
				}
			}
			notifier.OnNewEvent(nil, uid, types.StreamPosition(pos))
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
