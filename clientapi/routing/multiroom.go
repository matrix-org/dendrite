package routing

import (
	"io"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

func PostMultiroom(
	req *http.Request,
	device *api.Device,
	producer *producers.SyncAPIProducer,
	dataType string,
) util.JSONResponse {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithError(err).Errorf("failed to read request body")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	canonicalB, err := gomatrixserverlib.CanonicalJSON(b)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body is not valid canonical JSON." + err.Error()),
		}
	}
	err = producer.SendMultiroom(req.Context(), device.UserID, dataType, canonicalB)
	if err != nil {
		log.WithError(err).Errorf("failed to send multiroomcast")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
