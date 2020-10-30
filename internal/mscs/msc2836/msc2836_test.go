package msc2836_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	nethttputil "net/http/httputil"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/mscs/msc2836"
	"github.com/matrix-org/dendrite/internal/setup"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
)

// Basic sanity check of MSC2836 logic. Injects a thread that looks like:
//   A
//   |
//   B
//  / \
// C   D
//    /|\
//   E F G
//   |
//   H
// And makes sure POST /relationships works with various parameters
func TestMSC2836(t *testing.T) {
	router := injectEvents(t)
	externalServ := &http.Server{
		Addr:         string(":8009"),
		WriteTimeout: 60 * time.Second,
		Handler:      router,
	}
	defer externalServ.Shutdown(context.TODO())
	go func() {
		err := externalServ.ListenAndServe()
		if err != nil {
			t.Logf("ListenAndServe: %s", err)
		}
	}()
	// wait to listen on the port
	time.Sleep(500 * time.Millisecond)

	res := postRelationships(t, "alice", &msc2836.EventRelationshipRequest{
		EventID: "$invalid",
	})
	if res.StatusCode != 403 {
		out, _ := nethttputil.DumpResponse(res, true)
		t.Fatalf("failed to perform request: %s", string(out))
	}
}

func postRelationships(t *testing.T, accessToken string, req *msc2836.EventRelationshipRequest) *http.Response {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %s", err)
	}
	httpReq, err := http.NewRequest(
		"POST", "http://localhost:8009/_matrix/client/unstable/event_relationships",
		bytes.NewBuffer(data),
	)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	if err != nil {
		t.Fatalf("failed to prepare request: %s", err)
	}
	res, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("failed to do request: %s", err)
	}
	return res
}

type testUserAPI struct {
	accessTokens map[string]userapi.Device
}

func (u *testUserAPI) InputAccountData(ctx context.Context, req *userapi.InputAccountDataRequest, res *userapi.InputAccountDataResponse) error {
	return nil
}
func (u *testUserAPI) PerformAccountCreation(ctx context.Context, req *userapi.PerformAccountCreationRequest, res *userapi.PerformAccountCreationResponse) error {
	return nil
}
func (u *testUserAPI) PerformPasswordUpdate(ctx context.Context, req *userapi.PerformPasswordUpdateRequest, res *userapi.PerformPasswordUpdateResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceCreation(ctx context.Context, req *userapi.PerformDeviceCreationRequest, res *userapi.PerformDeviceCreationResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceDeletion(ctx context.Context, req *userapi.PerformDeviceDeletionRequest, res *userapi.PerformDeviceDeletionResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceUpdate(ctx context.Context, req *userapi.PerformDeviceUpdateRequest, res *userapi.PerformDeviceUpdateResponse) error {
	return nil
}
func (u *testUserAPI) PerformAccountDeactivation(ctx context.Context, req *userapi.PerformAccountDeactivationRequest, res *userapi.PerformAccountDeactivationResponse) error {
	return nil
}
func (u *testUserAPI) QueryProfile(ctx context.Context, req *userapi.QueryProfileRequest, res *userapi.QueryProfileResponse) error {
	return nil
}
func (u *testUserAPI) QueryAccessToken(ctx context.Context, req *userapi.QueryAccessTokenRequest, res *userapi.QueryAccessTokenResponse) error {
	dev, ok := u.accessTokens[req.AccessToken]
	if !ok {
		res.Err = fmt.Errorf("unknown token")
		return nil
	}
	res.Device = &dev
	return nil
}
func (u *testUserAPI) QueryDevices(ctx context.Context, req *userapi.QueryDevicesRequest, res *userapi.QueryDevicesResponse) error {
	return nil
}
func (u *testUserAPI) QueryAccountData(ctx context.Context, req *userapi.QueryAccountDataRequest, res *userapi.QueryAccountDataResponse) error {
	return nil
}
func (u *testUserAPI) QueryDeviceInfos(ctx context.Context, req *userapi.QueryDeviceInfosRequest, res *userapi.QueryDeviceInfosResponse) error {
	return nil
}
func (u *testUserAPI) QuerySearchProfiles(ctx context.Context, req *userapi.QuerySearchProfilesRequest, res *userapi.QuerySearchProfilesResponse) error {
	return nil
}

func injectEvents(t *testing.T) *mux.Router {
	t.Helper()
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = "localhost"
	cfg.MSCs.Database.ConnectionString = "file:msc2836_test.db"
	cfg.MSCs.MSCs = []string{"msc2836"}
	base := &setup.BaseDendrite{
		Cfg:                cfg,
		PublicClientAPIMux: mux.NewRouter().PathPrefix(httputil.PublicClientPathPrefix).Subrouter(),
	}
	nopUserAPI := &testUserAPI{
		accessTokens: make(map[string]userapi.Device),
	}
	nopUserAPI.accessTokens["alice"] = userapi.Device{
		AccessToken: "alice",
		DisplayName: "Alice",
		UserID:      "@alice:localhost",
	}
	nopUserAPI.accessTokens["bob"] = userapi.Device{
		AccessToken: "bob",
		DisplayName: "Bob",
		UserID:      "@bob:localhost",
	}

	err := msc2836.Enable(base, nopUserAPI)
	if err != nil {
		t.Fatalf("failed to enable MSC2836: %s", err)
	}
	var events []*gomatrixserverlib.HeaderedEvent
	for _, ev := range events {
		hooks.Run(hooks.KindNewEvent, ev)
	}
	return base.PublicClientAPIMux
}
