package httputil

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// UnmarshalJSONRequest into the given interface pointer. Returns an error JSON response if
// there was a problem unmarshalling. Calling this function consumes the request body.
func UnmarshalJSONRequest(req *http.Request, iface interface{}) *util.JSONResponse {
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(iface); err != nil {
		// TODO: We may want to suppress the Error() return in production? It's useful when
		// debugging because an error will be produced for both invalid/malformed JSON AND
		// valid JSON with incorrect types for values.
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	return nil
}

// LogThenError logs the given error then returns a matrix-compliant 500 internal server error response.
// This should be used to log fatal errors which require investigation. It should not be used
// to log client validation errors, etc.
func LogThenError(req *http.Request, err error) util.JSONResponse {
	util.GetLogger(req.Context()).WithError(err).Error("request failed")
	return jsonerror.InternalServerError()
}
