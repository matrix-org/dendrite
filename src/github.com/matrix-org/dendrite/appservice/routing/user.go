package routing

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
)

// URIToUIDResponse represents response to an AppService URI to User Id
// (/_matrix/app/r0/user?uri={url_encoded_uri}) request
type URIToUIDResponse struct {
	UserID string `json:"user_id"`
}

const pathPrefixUnstableThirdPartyUser = "/_matrix/app/unstable/thirdparty/user/"

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
	// contact the app services in parallel, rather than sequentially
	var wg sync.WaitGroup
	userIDs := make([]string, 0)
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Check all the fields associated with each application service
		if !appservice.IsInterestedInUserID(uri) {
			continue
		}
		wg.Add(1)
		// call the applicable application services in parallel
		go func(as config.ApplicationService, ids *[]string) {
			defer wg.Done()
			reqURL, err := url.Parse(as.URL + pathPrefixUnstableThirdPartyUser + as.ID +
				"?access_token=" + as.HSToken +
				"&fields=" + uri)
			if err != nil {
				return
			}
			resp, err := http.Get(reqURL.String())
			// take the first successful match and send that back to the user
			if err != nil {
				return
			}
			// decode the JSON to get the field we want
			body, _ := ioutil.ReadAll(resp.Body)
			respMap := map[string]interface{}{}
			if err := json.Unmarshal(body, &respMap); err != nil {
				return
			}
			if userID, ok := respMap["userid"].(string); ok {
				*ids = append(*ids, userID)
			}
		}(appservice, &userIDs)

	}

	wg.Wait()

	if len(userIDs) > 0 {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: URIToUIDResponse{UserID: userIDs[0]},
		}
	}

	return util.JSONResponse{
		Code: http.StatusNotFound,
		JSON: jsonerror.NotFound("URI not supported by app services"),
	}
}
