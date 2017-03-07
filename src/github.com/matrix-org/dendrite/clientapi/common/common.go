package common

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	"net/http"
)

// UnmarshalJSONRequest into the given interface pointer. Returns an error JSON response if
// there was a problem unmarshalling. Calling this function consumes the request body.
func UnmarshalJSONRequest(req *http.Request, iface interface{}) *util.JSONResponse {
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(iface); err != nil {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.NotJSON("The request body was not JSON"),
		}
	}
	return nil
}
