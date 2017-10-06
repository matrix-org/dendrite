package readers

import (
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	"net/http"
	"regexp"
)

const (
	maxUsernameLength = 254 // http://matrix.org/speculator/spec/HEAD/intro.html#user-identifiers TODO account for domain
)

var validUsernameRegex = regexp.MustCompile(`^[0-9a-zA-Z_\-./]+$`)

func validate(username string) *util.JSONResponse {
	if len(username) > maxUsernameLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'username' >%d characters", maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(username) {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("User ID can only contain characters a-z, 0-9, or '_-./'"),
		}
	} else if username[0] == '_' { // Regex checks its not a zero length string
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("User ID can't start with a '_'"),
		}
	}
	return nil
}

type availableResponse struct {
	Available bool `json:"available"`
}

func RegisterAvailable(
	req *http.Request,
	accountDB *accounts.Database,
) util.JSONResponse {
	username := req.URL.Query().Get("username")

	if username == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("Missing query parameter"),
		}
	}

	if resErr := validate(username); resErr != nil {
		return *resErr
	}

	if _, resErr := accountDB.GetProfileByLocalpart(req.Context(), username); resErr == nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("A different user ID has already been registered for this session"),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: availableResponse{
			Available: true,
		},
	}
}
