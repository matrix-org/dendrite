package routing

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
	"io/ioutil"
	"net/http"
)

// URIToUIDResponse represents response to an AppService URI to User Id
// (/_matrix/app/r0/user?uri={url_encoded_uri}) request
type URIToUIDResponse struct {
	UserID string `json:"user_id"`
}

// URIToUID implements `/_matrix/app/r0/user?uri={url_encoded_uri}`, which
// enables users to contact App Service users directly by taking an encoded
// URI and turning it into a Matrix ID on the homeserver.
// https://matrix.org/docs/spec/application_service/unstable.html#user-ids
func URIToUID(req *http.Request, cfg config.Dendrite) util.JSONResponse {
	uri := req.URL.Query().Get("uri")
	if uri == "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: nil,
		}
	}
	baseReqURL := "http://" + string(cfg.Matrix.ServerName) + "/_matrix/app/unstable/thirdparty/user/"
	//appServices := cfg.Derived.ApplicationServices
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Check all the fields associated with each application service
		if appservice.IsInterestedInUserID(uri) {
			// call the application service
			reqURL := baseReqURL + appservice.ID + "?access_token=" + appservice.HSToken +
				"&fields=" + uri
			resp, err := http.Get(reqURL)
			// take the first successful match and send that back to the user
			if err == nil {
				body, _ := ioutil.ReadAll(resp.Body)
				respMap := map[string]interface{}{}
				err := json.Unmarshal(body, &respMap)
				if err != nil {
					panic(err)
				}
				if userID, ok := respMap["userid"].(string); ok {
					return util.JSONResponse{
						Code: http.StatusOK,
						JSON: URIToUIDResponse{UserID: userID},
					}
				}
			}
		}
	}
	return util.JSONResponse{
		Code: http.StatusNotFound,
		JSON: jsonerror.NotFound("URI not supported by app services"),
	}
}
