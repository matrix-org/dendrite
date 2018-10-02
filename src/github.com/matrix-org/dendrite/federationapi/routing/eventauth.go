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

package routing

import (
	"context"
	"net/http"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetEventAuth returns event auth for the roomID and eventID
func GetEventAuth(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	query api.RoomserverQueryAPI,
	roomID string,
	eventID string,
) util.JSONResponse {
	state, err := getState(ctx, request, query, roomID, eventID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: gomatrixserverlib.RespEventAuth{AuthEvents: state.AuthEvents},
	}
}
