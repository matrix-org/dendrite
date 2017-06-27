// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package readers

import (
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"net/http"
	"strings"
)

// DirectoryRoom looks up a room alias
func DirectoryRoom(
	req *http.Request,
	device *authtypes.Device,
	roomAlias string,
	federation *gomatrixserverlib.FederationClient,
	cfg *config.Dendrite,
) util.JSONResponse {
	domain, err := domainFromID(roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	if domain == cfg.Matrix.ServerName {
		// TODO: Implement lookup up local room aliases.
		panic(fmt.Errorf("Looking up local room aliases is not implemented"))
	} else {
		resp, err := federation.LookupRoomAlias(domain, roomAlias)
		if err != nil {
			switch x := err.(type) {
			case gomatrix.HTTPError:
				if x.Code == 404 {
					return util.JSONResponse{
						Code: 404,
						JSON: jsonerror.NotFound("Room alias not found"),
					}
				}
			}
			// TODO: Return 502 if the remote server errored.
			// TODO: Return 504 if the remote server timed out.
			return httputil.LogThenError(req, err)
		}

		return util.JSONResponse{
			Code: 200,
			JSON: resp,
		}
	}
}

// domainFromID returns everything after the first ":" character to extract
// the domain part of a matrix ID.
// TODO: duplicated from gomatrixserverlib.
func domainFromID(id string) (gomatrixserverlib.ServerName, error) {
	// IDs have the format: SIGIL LOCALPART ":" DOMAIN
	// Split on the first ":" character since the domain can contain ":"
	// characters.
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		// The ID must have a ":" character.
		return "", fmt.Errorf("invalid ID: %q", id)
	}
	// Return everything after the first ":" character.
	return gomatrixserverlib.ServerName(parts[1]), nil
}
