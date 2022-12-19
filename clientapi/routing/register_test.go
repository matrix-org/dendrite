// Copyright 2017 Andrew Morgan <andrew@amorgan.xyz>
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

package routing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var (
	// Registration Flows that the server allows.
	allowedFlows = []authtypes.Flow{
		{
			Stages: []authtypes.LoginType{
				authtypes.LoginType("stage1"),
				authtypes.LoginType("stage2"),
			},
		},
		{
			Stages: []authtypes.LoginType{
				authtypes.LoginType("stage1"),
				authtypes.LoginType("stage3"),
			},
		},
	}
)

// Should return true as we're completing all the stages of a single flow in
// order.
func TestFlowCheckingCompleteFlowOrdered(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage1"),
		authtypes.LoginType("stage3"),
	}

	if !checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be true.")
	}
}

// Should return false as all stages in a single flow need to be completed.
func TestFlowCheckingStagesFromDifferentFlows(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage2"),
		authtypes.LoginType("stage3"),
	}

	if checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be false.")
	}
}

// Should return true as we're completing all the stages from a single flow, as
// well as some extraneous stages.
func TestFlowCheckingCompleteOrderedExtraneous(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage1"),
		authtypes.LoginType("stage3"),
		authtypes.LoginType("stage4"),
		authtypes.LoginType("stage5"),
	}
	if !checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be true.")
	}
}

// Should return false as we're submitting an empty flow.
func TestFlowCheckingEmptyFlow(t *testing.T) {
	testFlow := []authtypes.LoginType{}
	if checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be false.")
	}
}

// Should return false as we've completed a stage that isn't in any allowed flow.
func TestFlowCheckingInvalidStage(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage8"),
	}
	if checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be false.")
	}
}

// Should return true as we complete all stages of an allowed flow, though out
// of order, as well as extraneous stages.
func TestFlowCheckingExtraneousUnordered(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage5"),
		authtypes.LoginType("stage4"),
		authtypes.LoginType("stage3"),
		authtypes.LoginType("stage2"),
		authtypes.LoginType("stage1"),
	}
	if !checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be true.")
	}
}

// Should return false as we're providing fewer stages than are required.
func TestFlowCheckingShortIncorrectInput(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage8"),
	}
	if checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be false.")
	}
}

// Should return false as we're providing different stages than are required.
func TestFlowCheckingExtraneousIncorrectInput(t *testing.T) {
	testFlow := []authtypes.LoginType{
		authtypes.LoginType("stage8"),
		authtypes.LoginType("stage9"),
		authtypes.LoginType("stage10"),
		authtypes.LoginType("stage11"),
	}
	if checkFlowCompleted(testFlow, allowedFlows) {
		t.Error("Incorrect registration flow verification: ", testFlow, ", from allowed flows: ", allowedFlows, ". Should be false.")
	}
}

// Completed flows stages should always be a valid slice header.
// TestEmptyCompletedFlows checks that sessionsDict returns a slice & not nil.
func TestEmptyCompletedFlows(t *testing.T) {
	fakeEmptySessions := newSessionsDict()
	fakeSessionID := "aRandomSessionIDWhichDoesNotExist"
	ret := fakeEmptySessions.getCompletedStages(fakeSessionID)

	// check for []
	if ret == nil || len(ret) != 0 {
		t.Error("Empty Completed Flow Stages should be a empty slice: returned ", ret, ". Should be []")
	}
}

// This method tests validation of the provided Application Service token and
// username that they're registering
func TestValidationOfApplicationServices(t *testing.T) {
	// Set up application service namespaces
	regex := "@_appservice_.*"
	regexp, err := regexp.Compile(regex)
	if err != nil {
		t.Errorf("Error compiling regex: %s", regex)
	}

	fakeNamespace := config.ApplicationServiceNamespace{
		Exclusive:    true,
		Regex:        regex,
		RegexpObject: regexp,
	}

	// Create a fake application service
	fakeID := "FakeAS"
	fakeSenderLocalpart := "_appservice_bot"
	fakeApplicationService := config.ApplicationService{
		ID:              fakeID,
		URL:             "null",
		ASToken:         "1234",
		HSToken:         "4321",
		SenderLocalpart: fakeSenderLocalpart,
		NamespaceMap: map[string][]config.ApplicationServiceNamespace{
			"users": {fakeNamespace},
		},
	}

	// Set up a config
	fakeConfig := &config.Dendrite{}
	fakeConfig.Defaults(config.DefaultOpts{
		Generate:   true,
		Monolithic: true,
	})
	fakeConfig.Global.ServerName = "localhost"
	fakeConfig.ClientAPI.Derived.ApplicationServices = []config.ApplicationService{fakeApplicationService}

	// Access token is correct, user_id omitted so we are acting as SenderLocalpart
	asID, resp := validateApplicationService(&fakeConfig.ClientAPI, fakeSenderLocalpart, "1234")
	if resp != nil || asID != fakeID {
		t.Errorf("appservice should have validated and returned correct ID: %s", resp.JSON)
	}

	// Access token is incorrect, user_id omitted so we are acting as SenderLocalpart
	asID, resp = validateApplicationService(&fakeConfig.ClientAPI, fakeSenderLocalpart, "xxxx")
	if resp == nil || asID == fakeID {
		t.Errorf("access_token should have been marked as invalid")
	}

	// Access token is correct, acting as valid user_id
	asID, resp = validateApplicationService(&fakeConfig.ClientAPI, "_appservice_bob", "1234")
	if resp != nil || asID != fakeID {
		t.Errorf("access_token and user_id should've been valid: %s", resp.JSON)
	}

	// Access token is correct, acting as invalid user_id
	asID, resp = validateApplicationService(&fakeConfig.ClientAPI, "_something_else", "1234")
	if resp == nil || asID == fakeID {
		t.Errorf("user_id should not have been valid: @_something_else:localhost")
	}
}

func TestSessionCleanUp(t *testing.T) {
	s := newSessionsDict()

	t.Run("session is cleaned up after a while", func(t *testing.T) {
		// t.Parallel()
		dummySession := "helloWorld"
		// manually added, as s.addParams() would start the timer with the default timeout
		s.params[dummySession] = registerRequest{Username: "Testing"}
		s.startTimer(time.Millisecond, dummySession)
		time.Sleep(time.Millisecond * 50)
		if data, ok := s.getParams(dummySession); ok {
			t.Errorf("expected session to be deleted: %+v", data)
		}
	})

	t.Run("session is deleted, once the registration completed", func(t *testing.T) {
		// t.Parallel()
		dummySession := "helloWorld2"
		s.startTimer(time.Minute, dummySession)
		s.deleteSession(dummySession)
		if data, ok := s.getParams(dummySession); ok {
			t.Errorf("expected session to be deleted: %+v", data)
		}
	})

	t.Run("session timer is restarted after second call", func(t *testing.T) {
		// t.Parallel()
		dummySession := "helloWorld3"
		// the following will start a timer with the default timeout of 5min
		s.addParams(dummySession, registerRequest{Username: "Testing"})
		s.addCompletedSessionStage(dummySession, authtypes.LoginTypeRecaptcha)
		s.addCompletedSessionStage(dummySession, authtypes.LoginTypeDummy)
		s.addDeviceToDelete(dummySession, "dummyDevice")
		s.getCompletedStages(dummySession)
		// reset the timer with a lower timeout
		s.startTimer(time.Millisecond, dummySession)
		time.Sleep(time.Millisecond * 50)
		if data, ok := s.getParams(dummySession); ok {
			t.Errorf("expected session to be deleted: %+v", data)
		}
		if _, ok := s.timer[dummySession]; ok {
			t.Error("expected timer to be delete")
		}
		if _, ok := s.sessions[dummySession]; ok {
			t.Error("expected session to be delete")
		}
		if _, ok := s.getDeviceToDelete(dummySession); ok {
			t.Error("expected session to device to be delete")
		}
	})
}

func Test_validateUsername(t *testing.T) {
	tooLongUsername := strings.Repeat("a", maxUsernameLength)
	tests := []struct {
		name      string
		localpart string
		domain    gomatrixserverlib.ServerName
		want      *util.JSONResponse
	}{
		{
			name:      "empty username",
			localpart: "",
			domain:    "localhost",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./='"),
			},
		},
		{
			name:      "invalid username",
			localpart: "INVALIDUSERNAME",
			domain:    "localhost",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./='"),
			},
		},
		{
			name:      "username too long",
			localpart: tooLongUsername,
			domain:    "localhost",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON(fmt.Sprintf("%q exceeds the maximum length of %d characters", fmt.Sprintf("@%s:%s", tooLongUsername, "localhost"), maxUsernameLength)),
			},
		},
		{
			name:      "localpart starting with an underscore",
			localpart: "_notvalid",
			domain:    "localhost",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("Username cannot start with a '_'"),
			},
		},
		{
			name:      "valid username",
			localpart: "valid",
			domain:    "localhost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateUsername(tt.localpart, tt.domain); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateUsername() = %v, want %v", got, tt.want)
			}
			if got := validateApplicationServiceUsername(tt.localpart, tt.domain); !reflect.DeepEqual(got, tt.want) {
				if got != nil && got.JSON != jsonerror.InvalidUsername("Username cannot start with a '_'") {
					t.Errorf("validateUsername() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_validatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     *util.JSONResponse
	}{
		{
			name: "no password supplied",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
			},
		},
		{
			name:     "password too short",
			password: "shortpw",
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
			},
		},
		{
			name:     "password too long",
			password: strings.Repeat("a", maxPasswordLength+1),
			want: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON(fmt.Sprintf("'password' >%d characters", maxPasswordLength)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatePassword(tt.password); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validatePassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_register(t *testing.T) {
	testCases := []struct {
		name                 string
		kind                 string
		password             string
		username             string
		loginType            string
		forceEmpty           bool
		registrationDisabled bool
		guestsDisabled       bool
		wantResponse         util.JSONResponse
	}{
		{
			name:           "disallow guests",
			kind:           "guest",
			guestsDisabled: true,
			wantResponse: util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(`Guest registration is disabled on "test"`),
			},
		},
		{
			name: "allow guests",
			kind: "guest",
		},
		{
			name:      "unknown login type",
			loginType: "im.not.known",
			wantResponse: util.JSONResponse{
				Code: http.StatusNotImplemented,
				JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
			},
		},
		{
			name:                 "disabled registration",
			registrationDisabled: true,
			wantResponse: util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(`Registration is disabled on "test"`),
			},
		},
		{
			name:       "successful registration, numeric ID",
			username:   "",
			password:   "someRandomPassword",
			forceEmpty: true,
		},
		{
			name:     "successful registration",
			username: "success",
		},
		{
			name:     "failing registration - user already exists",
			username: "success",
			wantResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UserInUse("Desired user ID is already taken."),
			},
		},
		{
			name:     "successful registration",
			username: "LOWERCASED", // this is going to be lower-cased
		},
		{
			name:     "invalid username",
			username: "#totalyNotValid",
			wantResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./='"),
			},
		},
		{
			name:     "numeric username is forbidden",
			username: "1337",
			wantResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("Numeric user IDs are reserved"),
			},
		},
		{
			name:       "can't register without password",
			username:   "helloworld",
			password:   "",
			forceEmpty: true,
			wantResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
			},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, baseClose := testrig.CreateBaseDendrite(t, dbType)
		defer baseClose()

		rsAPI := roomserver.NewInternalAPI(base)
		keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, nil, rsAPI)
		userAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, keyAPI, rsAPI, nil)
		keyAPI.SetUserAPI(userAPI)

		if err := base.Cfg.Derive(); err != nil {
			t.Fatalf("failed to derive config: %s", err)
		}

		for _, tc := range testCases {
			if tc.kind == "" {
				tc.kind = "user"
			}
			if tc.password == "" && !tc.forceEmpty {
				tc.password = "someRandomPassword"
			}
			if tc.username == "" && !tc.forceEmpty {
				tc.username = "valid"
			}
			if tc.loginType == "" {
				tc.loginType = "m.login.dummy"
			}

			reg := registerRequest{
				Password: tc.password,
				Username: tc.username,
			}

			body := &bytes.Buffer{}

			base.Cfg.ClientAPI.RegistrationDisabled = tc.registrationDisabled
			base.Cfg.ClientAPI.GuestsDisabled = tc.guestsDisabled

			err := json.NewEncoder(body).Encode(reg)
			if err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/?kind=%s", tc.kind), body)

			resp := Register(req, userAPI, &base.Cfg.ClientAPI)
			t.Logf("Resp: %+v", resp)

			// The first request should return a userInteractiveResponse
			switch r := resp.JSON.(type) {
			case userInteractiveResponse:
				// Check that the flows are the ones we configured
				if !reflect.DeepEqual(r.Flows, base.Cfg.Derived.Registration.Flows) {
					t.Fatalf("unexpected registration flows: %+v, want %+v", r.Flows, base.Cfg.Derived.Registration.Flows)
				}
			case *jsonerror.MatrixError:
				if !reflect.DeepEqual(tc.wantResponse, resp) {
					t.Fatalf("(%s), unexpected response: %+v, want: %+v", tc.name, resp, tc.wantResponse)
				}
				continue
			case registerResponse:
				// this should only be possible on guest user registration, never for normal users
				if tc.kind != "guest" {
					t.Fatalf("got register response on first request: %+v", r)
				}
				// assert we've got a UserID, AccessToken and DeviceID
				if r.UserID == "" {
					t.Fatalf("missing userID in response")
				}
				if r.AccessToken == "" {
					t.Fatalf("missing accessToken in response")
				}
				if r.DeviceID == "" {
					t.Fatalf("missing deviceID in response")
				}
				continue
			default:
				t.Logf("Got response: %T", resp.JSON)
			}

			// If we reached this, we should have received a UIA response
			uia, ok := resp.JSON.(userInteractiveResponse)
			if !ok {
				t.Fatalf("did not receive a userInteractiveResponse: %T", resp.JSON)
			}
			t.Logf("%+v", uia)

			// Register the user
			reg.Auth = authDict{
				Type:    authtypes.LoginType(tc.loginType),
				Session: uia.Session,
			}
			dummy := "dummy"
			reg.DeviceID = &dummy
			reg.InitialDisplayName = &dummy
			reg.Type = authtypes.LoginType(tc.loginType)

			err = json.NewEncoder(body).Encode(reg)
			if err != nil {
				t.Fatal(err)
			}

			req = httptest.NewRequest(http.MethodPost, "/", body)

			resp = Register(req, userAPI, &base.Cfg.ClientAPI)

			switch resp.JSON.(type) {
			case *jsonerror.MatrixError:
				if !reflect.DeepEqual(tc.wantResponse, resp) {
					t.Fatalf("unexpected response: %+v, want: %+v", resp, tc.wantResponse)
				}
				continue
			}

			rr, ok := resp.JSON.(registerResponse)
			if !ok {
				t.Fatalf("expected a registerresponse, got %T", resp.JSON)
			}

			// validate the response
			if tc.forceEmpty {
				// when not supplying a username, one will be generated. Given this _SHOULD_ be
				// the second user, set the username accordingly
				reg.Username = "2"
			}
			wantUserID := strings.ToLower(fmt.Sprintf("@%s:%s", reg.Username, "test"))
			if wantUserID != rr.UserID {
				t.Fatalf("unexpected userID: %s, want %s", rr.UserID, wantUserID)
			}
			if rr.DeviceID != *reg.DeviceID {
				t.Fatalf("unexpected deviceID: %s, want %s", rr.DeviceID, *reg.DeviceID)
			}
			if rr.AccessToken == "" {
				t.Fatalf("missing accessToken in response")
			}
		}
	})
}
