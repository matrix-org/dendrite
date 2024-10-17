package internal

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

func Test_validatePassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		wantError error
		wantJSON  *util.JSONResponse
	}{
		{
			name:      "password too short",
			password:  "shortpw",
			wantError: ErrPasswordWeak,
			wantJSON:  &util.JSONResponse{Code: http.StatusBadRequest, JSON: spec.WeakPassword(ErrPasswordWeak.Error())},
		},
		{
			name:      "password too long",
			password:  strings.Repeat("a", maxPasswordLength+1),
			wantError: ErrPasswordTooLong,
			wantJSON:  &util.JSONResponse{Code: http.StatusBadRequest, JSON: spec.BadJSON(ErrPasswordTooLong.Error())},
		},
		{
			name:     "password OK",
			password: util.RandomString(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := ValidatePassword(tt.password)
			if !reflect.DeepEqual(gotErr, tt.wantError) {
				t.Errorf("validatePassword() = %v, wantError %v", gotErr, tt.wantError)
			}

			if got := PasswordResponse(gotErr); !reflect.DeepEqual(got, tt.wantJSON) {
				t.Errorf("validatePassword() = %v, wantJSON %v", got, tt.wantJSON)
			}
		})
	}
}

func Test_validateUsername(t *testing.T) {
	tooLongUsername := strings.Repeat("a", maxUsernameLength)
	tests := []struct {
		name      string
		localpart string
		domain    spec.ServerName
		wantErr   error
		wantJSON  *util.JSONResponse
	}{
		{
			name:      "empty username",
			localpart: "",
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "invalid username",
			localpart: "INVALIDUSERNAME",
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "username too long",
			localpart: tooLongUsername,
			domain:    "localhost",
			wantErr:   ErrUsernameTooLong,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON(ErrUsernameTooLong.Error()),
			},
		},
		{
			name:      "localpart starting with an underscore",
			localpart: "_notvalid",
			domain:    "localhost",
			wantErr:   ErrUsernameUnderscore,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameUnderscore.Error()),
			},
		},
		{
			name:      "valid username",
			localpart: "valid",
			domain:    "localhost",
		},
		{
			name:      "complex username",
			localpart: "f00_bar-baz.=40/",
			domain:    "localhost",
		},
		{
			name:      "rejects emoji username 💥",
			localpart: "💥",
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "special characters are allowed",
			localpart: "/dev/null",
			domain:    "localhost",
		},
		{
			name:      "special characters are allowed 2",
			localpart: "i_am_allowed=1",
			domain:    "localhost",
		},
		{
			name:      "special characters are allowed 3",
			localpart: "+55555555555",
			domain:    "localhost",
		},
		{
			name:      "not all special characters are allowed",
			localpart: "notallowed#", // contains #
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "not all special characters are allowed 2",
			localpart: "<notallowed", // contains <
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "username containing numbers",
			localpart: "hello1337",
			domain:    "localhost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := ValidateUsername(tt.localpart, tt.domain)
			if !reflect.DeepEqual(gotErr, tt.wantErr) {
				t.Errorf("ValidateUsername() = %v, wantErr %v", gotErr, tt.wantErr)
			}
			if gotJSON := UsernameResponse(gotErr); !reflect.DeepEqual(gotJSON, tt.wantJSON) {
				t.Errorf("UsernameResponse() = %v, wantJSON %v", gotJSON, tt.wantJSON)
			}

			// Application services are allowed usernames starting with an underscore
			if tt.wantErr == ErrUsernameUnderscore {
				return
			}
			gotErr = ValidateApplicationServiceUsername(tt.localpart, tt.domain)
			if !reflect.DeepEqual(gotErr, tt.wantErr) {
				t.Errorf("ValidateUsername() = %v, wantErr %v", gotErr, tt.wantErr)
			}
			if gotJSON := UsernameResponse(gotErr); !reflect.DeepEqual(gotJSON, tt.wantJSON) {
				t.Errorf("UsernameResponse() = %v, wantJSON %v", gotJSON, tt.wantJSON)
			}
		})
	}
}

// This method tests validation of the provided Application Service token and
// username that they're registering
func TestValidateApplicationServiceRequest(t *testing.T) {
	// Create a fake application service
	regex := "@_appservice_.*"
	fakeNamespace := config.ApplicationServiceNamespace{
		Exclusive:    true,
		Regex:        regex,
		RegexpObject: regexp.MustCompile(regex),
	}
	fakeSenderLocalpart := "_appservice_bot"
	fakeApplicationService := config.ApplicationService{
		ID:              "FakeAS",
		URL:             "null",
		ASToken:         "1234",
		HSToken:         "4321",
		SenderLocalpart: fakeSenderLocalpart,
		NamespaceMap: map[string][]config.ApplicationServiceNamespace{
			"users": {fakeNamespace},
		},
	}

	// Create a second fake application service where userIDs ending in
	// "_overlap" overlap with the first.
	regex = "@_.*_overlap"
	fakeNamespace = config.ApplicationServiceNamespace{
		Exclusive:    true,
		Regex:        regex,
		RegexpObject: regexp.MustCompile(regex),
	}
	fakeApplicationServiceOverlap := config.ApplicationService{
		ID:              "FakeASOverlap",
		URL:             fakeApplicationService.URL,
		ASToken:         fakeApplicationService.ASToken,
		HSToken:         fakeApplicationService.HSToken,
		SenderLocalpart: "_appservice_bot_overlap",
		NamespaceMap: map[string][]config.ApplicationServiceNamespace{
			"users": {fakeNamespace},
		},
	}

	// Set up a config
	fakeConfig := &config.Dendrite{}
	fakeConfig.Defaults(config.DefaultOpts{
		Generate: true,
	})
	fakeConfig.Global.ServerName = "localhost"
	fakeConfig.ClientAPI.Derived.ApplicationServices = []config.ApplicationService{fakeApplicationService, fakeApplicationServiceOverlap}

	tests := []struct {
		name      string
		localpart string
		asToken   string
		wantError bool
		wantASID  string
	}{
		// Access token is correct, userID omitted so we are acting as SenderLocalpart
		{
			name:      "correct access token but omitted userID",
			localpart: fakeSenderLocalpart,
			asToken:   fakeApplicationService.ASToken,
			wantError: false,
			wantASID:  fakeApplicationService.ID,
		},
		// Access token is incorrect, userID omitted so we are acting as SenderLocalpart
		{
			name:      "incorrect access token but omitted userID",
			localpart: fakeSenderLocalpart,
			asToken:   "xxxx",
			wantError: true,
			wantASID:  "",
		},
		// Access token is correct, acting as valid userID
		{
			name:      "correct access token and valid userID",
			localpart: "_appservice_bob",
			asToken:   fakeApplicationService.ASToken,
			wantError: false,
			wantASID:  fakeApplicationService.ID,
		},
		// Access token is correct, acting as invalid userID
		{
			name:      "correct access token but invalid userID",
			localpart: "_something_else",
			asToken:   fakeApplicationService.ASToken,
			wantError: true,
			wantASID:  "",
		},
		// Access token is correct, acting as userID that matches two exclusive namespaces
		{
			name:      "correct access token but non-exclusive userID",
			localpart: "_appservice_overlap",
			asToken:   fakeApplicationService.ASToken,
			wantError: true,
			wantASID:  "",
		},
		// Access token is correct, acting as matching userID that is too long
		{
			name:      "correct access token but too long userID",
			localpart: "_appservice_" + strings.Repeat("a", maxUsernameLength),
			asToken:   fakeApplicationService.ASToken,
			wantError: true,
			wantASID:  "",
		},
		// Access token is correct, acting as userID that matches but is invalid
		{
			name:      "correct access token and matching but invalid userID",
			localpart: "@_appservice_bob::",
			asToken:   fakeApplicationService.ASToken,
			wantError: true,
			wantASID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotASID, gotResp := ValidateApplicationServiceRequest(&fakeConfig.ClientAPI, tt.localpart, tt.asToken)
			if tt.wantError && gotResp == nil {
				t.Error("expected an error, but succeeded")
			}
			if !tt.wantError && gotResp != nil {
				t.Errorf("expected success, but returned error: %v", *gotResp)
			}
			if gotASID != tt.wantASID {
				t.Errorf("returned '%s', but expected '%s'", gotASID, tt.wantASID)
			}
		})
	}
}
