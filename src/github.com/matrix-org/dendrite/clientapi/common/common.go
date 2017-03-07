package common

import (
	"encoding/json"
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	"net/http"
	"strings"
)

// UserID represents a parsed User ID string
type UserID struct {
	Domain    string
	Localpart string
}

// UserIDFromString creates a UserID from an input string. Returns an error if
// the string is not a valid user ID.
func UserIDFromString(id string) (uid UserID, err error) {
	// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/types.py#L92
	if len(id) == 0 || id[0] != '@' {
		err = fmt.Errorf("user id must start with '@'")
		return
	}
	parts := strings.SplitN(id[1:], ":", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("user id must be in the form @localpart:domain")
		return
	}
	uid.Localpart = parts[0]
	uid.Domain = parts[1]
	return
}

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
