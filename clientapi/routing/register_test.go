// Copyright 2024 New Vector Ltd.
// Copyright 2017 Andrew Morgan <andrew@amorgan.xyz>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
	"github.com/element-hq/dendrite/userapi"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
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
		Generate:       true,
		SingleDatabase: true,
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
		enableRecaptcha      bool
		captchaBody          string
		// in case of an error, the expected response
		wantErrorResponse util.JSONResponse
		// in case of success, the expected username assigned
		wantUsername string
	}{
		{
			name:           "disallow guests",
			kind:           "guest",
			guestsDisabled: true,
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden(`Guest registration is disabled on "test"`),
			},
		},
		{
			name:         "allow guests",
			kind:         "guest",
			wantUsername: "1",
		},
		{
			name:      "unknown login type",
			loginType: "im.not.known",
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusNotImplemented,
				JSON: spec.Unknown("unknown/unimplemented auth type"),
			},
		},
		{
			name:                 "disabled registration",
			registrationDisabled: true,
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden(`Registration is disabled on "test"`),
			},
		},
		{
			name:         "successful registration, numeric ID",
			username:     "",
			password:     "someRandomPassword",
			forceEmpty:   true,
			wantUsername: "2",
		},
		{
			name:     "successful registration",
			username: "success",
		},
		{
			name:         "successful registration, sequential numeric ID",
			username:     "",
			password:     "someRandomPassword",
			forceEmpty:   true,
			wantUsername: "3",
		},
		{
			name:     "failing registration - user already exists",
			username: "success",
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.UserInUse("Desired user ID is already taken."),
			},
		},
		{
			name:     "successful registration uppercase username",
			username: "LOWERCASED", // this is going to be lower-cased
		},
		{
			name:              "invalid username",
			username:          "#totalyNotValid",
			wantErrorResponse: *internal.UsernameResponse(internal.ErrUsernameInvalid),
		},
		{
			name:     "numeric username is forbidden",
			username: "1337",
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername("Numeric user IDs are reserved"),
			},
		},
		{
			name:      "disabled recaptcha login",
			loginType: authtypes.LoginTypeRecaptcha,
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Unknown(ErrCaptchaDisabled.Error()),
			},
		},
		{
			name:            "enabled recaptcha, no response defined",
			enableRecaptcha: true,
			loginType:       authtypes.LoginTypeRecaptcha,
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON(ErrMissingResponse.Error()),
			},
		},
		{
			name:            "invalid captcha response",
			enableRecaptcha: true,
			loginType:       authtypes.LoginTypeRecaptcha,
			captchaBody:     `notvalid`,
			wantErrorResponse: util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: spec.BadJSON(ErrInvalidCaptcha.Error()),
			},
		},
		{
			name:            "valid captcha response",
			enableRecaptcha: true,
			loginType:       authtypes.LoginTypeRecaptcha,
			captchaBody:     `success`,
		},
		{
			name:              "captcha invalid from remote",
			enableRecaptcha:   true,
			loginType:         authtypes.LoginTypeRecaptcha,
			captchaBody:       `i should fail for other reasons`,
			wantErrorResponse: util.JSONResponse{Code: http.StatusInternalServerError, JSON: spec.InternalServerError{}},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()

		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.enableRecaptcha {
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if err := r.ParseForm(); err != nil {
							t.Fatal(err)
						}
						response := r.Form.Get("response")

						// Respond with valid JSON or no JSON at all to test happy/error cases
						switch response {
						case "success":
							json.NewEncoder(w).Encode(recaptchaResponse{Success: true})
						case "notvalid":
							json.NewEncoder(w).Encode(recaptchaResponse{Success: false})
						default:

						}
					}))
					defer srv.Close()
					cfg.ClientAPI.RecaptchaSiteVerifyAPI = srv.URL
				}

				if err := cfg.Derive(); err != nil {
					t.Fatalf("failed to derive config: %s", err)
				}

				cfg.ClientAPI.RecaptchaEnabled = tc.enableRecaptcha
				cfg.ClientAPI.RegistrationDisabled = tc.registrationDisabled
				cfg.ClientAPI.GuestsDisabled = tc.guestsDisabled

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
				err := json.NewEncoder(body).Encode(reg)
				if err != nil {
					t.Fatal(err)
				}

				req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/?kind=%s", tc.kind), body)

				resp := Register(req, userAPI, &cfg.ClientAPI)
				t.Logf("Resp: %+v", resp)

				// The first request should return a userInteractiveResponse
				switch r := resp.JSON.(type) {
				case userInteractiveResponse:
					// Check that the flows are the ones we configured
					if !reflect.DeepEqual(r.Flows, cfg.Derived.Registration.Flows) {
						t.Fatalf("unexpected registration flows: %+v, want %+v", r.Flows, cfg.Derived.Registration.Flows)
					}
				case spec.MatrixError:
					if !reflect.DeepEqual(tc.wantErrorResponse, resp) {
						t.Fatalf("(%s), unexpected response: %+v, want: %+v", tc.name, resp, tc.wantErrorResponse)
					}
					return
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
					// if an expected username is provided, assert that it is a match
					if tc.wantUsername != "" {
						wantUserID := strings.ToLower(fmt.Sprintf("@%s:%s", tc.wantUsername, "test"))
						if wantUserID != r.UserID {
							t.Fatalf("unexpected userID: %s, want %s", r.UserID, wantUserID)
						}
					}
					return
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

				if tc.captchaBody != "" {
					reg.Auth.Response = tc.captchaBody
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

				resp = Register(req, userAPI, &cfg.ClientAPI)

				switch rr := resp.JSON.(type) {
				case spec.InternalServerError, spec.MatrixError, util.JSONResponse:
					if !reflect.DeepEqual(tc.wantErrorResponse, resp) {
						t.Fatalf("unexpected response: %+v, want: %+v", resp, tc.wantErrorResponse)
					}
					return
				case registerResponse:
					// validate the response
					if tc.wantUsername != "" {
						// if an expected username is provided, assert that it is a match
						wantUserID := strings.ToLower(fmt.Sprintf("@%s:%s", tc.wantUsername, "test"))
						if wantUserID != rr.UserID {
							t.Fatalf("unexpected userID: %s, want %s", rr.UserID, wantUserID)
						}
					}
					if rr.DeviceID != *reg.DeviceID {
						t.Fatalf("unexpected deviceID: %s, want %s", rr.DeviceID, *reg.DeviceID)
					}
					if rr.AccessToken == "" {
						t.Fatalf("missing accessToken in response")
					}
				default:
					t.Fatalf("expected one of internalservererror, matrixerror, jsonresponse, registerresponse, got %T", resp.JSON)
				}
			})
		}
	})
}

func TestRegisterUserWithDisplayName(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		cfg.Global.ServerName = "server"

		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		deviceName, deviceID := "deviceName", "deviceID"
		expectedDisplayName := "DisplayName"
		response := completeRegistration(
			processCtx.Context(),
			userAPI,
			"user",
			"server",
			expectedDisplayName,
			"password",
			"",
			"localhost",
			"user agent",
			"session",
			false,
			&deviceName,
			&deviceID,
			api.AccountTypeAdmin,
		)

		assert.Equal(t, http.StatusOK, response.Code)

		profile, err := userAPI.QueryProfile(processCtx.Context(), "@user:server")
		assert.NoError(t, err)
		assert.Equal(t, expectedDisplayName, profile.DisplayName)
	})
}

func TestRegisterAdminUsingSharedSecret(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		natsInstance := jetstream.NATSInstance{}
		cfg.Global.ServerName = "server"
		sharedSecret := "dendritetest"
		cfg.ClientAPI.RegistrationSharedSecret = sharedSecret

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)

		expectedDisplayName := "rabbit"
		jsonStr := []byte(`{"admin":true,"mac":"24dca3bba410e43fe64b9b5c28306693bf3baa9f","nonce":"759f047f312b99ff428b21d581256f8592b8976e58bc1b543972dc6147e529a79657605b52d7becd160ff5137f3de11975684319187e06901955f79e5a6c5a79","password":"wonderland","username":"alice","displayname":"rabbit"}`)
		req, err := NewSharedSecretRegistrationRequest(io.NopCloser(bytes.NewBuffer(jsonStr)))
		assert.NoError(t, err)
		if err != nil {
			t.Fatalf("failed to read request: %s", err)
		}

		r := NewSharedSecretRegistration(sharedSecret)

		// force the nonce to be known
		r.nonces.Set(req.Nonce, true, cache.DefaultExpiration)

		_, err = r.IsValidMacLogin(req.Nonce, req.User, req.Password, req.Admin, req.MacBytes)
		assert.NoError(t, err)

		body := &bytes.Buffer{}
		err = json.NewEncoder(body).Encode(req)
		assert.NoError(t, err)
		ssrr := httptest.NewRequest(http.MethodPost, "/", body)

		response := handleSharedSecretRegistration(
			&cfg.ClientAPI,
			userAPI,
			r,
			ssrr,
		)
		assert.Equal(t, http.StatusOK, response.Code)

		profile, err := userAPI.QueryProfile(processCtx.Context(), "@alice:server")
		assert.NoError(t, err)
		assert.Equal(t, expectedDisplayName, profile.DisplayName)
	})
}
