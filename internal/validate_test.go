package internal

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
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
			wantJSON:  &util.JSONResponse{Code: http.StatusBadRequest, JSON: jsonerror.WeakPassword(ErrPasswordWeak.Error())},
		},
		{
			name:      "password too long",
			password:  strings.Repeat("a", maxPasswordLength+1),
			wantError: ErrPasswordTooLong,
			wantJSON:  &util.JSONResponse{Code: http.StatusBadRequest, JSON: jsonerror.BadJSON(ErrPasswordTooLong.Error())},
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
				t.Errorf("validatePassword() = %v, wantJSON %v", gotErr, tt.wantError)
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
		domain    gomatrixserverlib.ServerName
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
				JSON: jsonerror.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "invalid username",
			localpart: "INVALIDUSERNAME",
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername(ErrUsernameInvalid.Error()),
			},
		},
		{
			name:      "username too long",
			localpart: tooLongUsername,
			domain:    "localhost",
			wantErr:   ErrUsernameTooLong,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON(ErrUsernameTooLong.Error()),
			},
		},
		{
			name:      "localpart starting with an underscore",
			localpart: "_notvalid",
			domain:    "localhost",
			wantErr:   ErrUsernameUnderscore,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername(ErrUsernameUnderscore.Error()),
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
				JSON: jsonerror.InvalidUsername(ErrUsernameInvalid.Error()),
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
			name:      "not all special characters are allowed",
			localpart: "notallowed#", // contains #
			domain:    "localhost",
			wantErr:   ErrUsernameInvalid,
			wantJSON: &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername(ErrUsernameInvalid.Error()),
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
func TestValidationOfApplicationServices(t *testing.T) {
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
	regex = "@.*_overlap"
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
		Generate:   true,
		Monolithic: true,
	})
	fakeConfig.Global.ServerName = "localhost"
	fakeConfig.ClientAPI.Derived.ApplicationServices = []config.ApplicationService{fakeApplicationService, fakeApplicationServiceOverlap}

	// Access token is correct, user_id omitted so we are acting as SenderLocalpart
	asID, resp := ValidateApplicationService(&fakeConfig.ClientAPI, fakeSenderLocalpart, fakeApplicationService.ASToken)
	if resp != nil || asID != fakeApplicationService.ID {
		t.Errorf("appservice should have validated and returned correct ID: %s", resp.JSON)
	}

	// Access token is incorrect, user_id omitted so we are acting as SenderLocalpart
	asID, resp = ValidateApplicationService(&fakeConfig.ClientAPI, fakeSenderLocalpart, "xxxx")
	if resp == nil || asID == fakeApplicationService.ID {
		t.Errorf("access_token should have been marked as invalid")
	}

	// Access token is correct, acting as valid user_id
	asID, resp = ValidateApplicationService(&fakeConfig.ClientAPI, "_appservice_bob", fakeApplicationService.ASToken)
	if resp != nil || asID != fakeApplicationService.ID {
		t.Errorf("access_token and user_id should've been valid: %s", resp.JSON)
	}

	// Access token is correct, acting as invalid user_id
	asID, resp = ValidateApplicationService(&fakeConfig.ClientAPI, "_something_else", fakeApplicationService.ASToken)
	if resp == nil || asID == fakeApplicationService.ID {
		t.Errorf("user_id should not have been valid: @_something_else:localhost")
	}

	// Access token is correct, acting as user_id that matches two exclusive namespaces
	asID, resp = ValidateApplicationService(&fakeConfig.ClientAPI, "_appservice_overlap", fakeApplicationService.ASToken)
	if resp == nil || asID == fakeApplicationService.ID {
		t.Errorf("user_id should not have been valid: @_appservice_overlap:localhost")
	}

	// Access token is correct, acting as matching user_id that is too long
	asID, resp = ValidateApplicationService(
		&fakeConfig.ClientAPI,
		"_appservice_"+strings.Repeat("a", maxUsernameLength),
		fakeApplicationService.ASToken,
	)
	if resp == nil || asID == fakeApplicationService.ID {
		t.Errorf("user_id exceeding maxUsernameLength should not have been valid")
	}

	// Access token is correct, acting as user_id that matches but is invalid
	asID, resp = ValidateApplicationService(&fakeConfig.ClientAPI, "@_appservice_bob::", fakeApplicationService.ASToken)
	if resp == nil || asID == fakeApplicationService.ID {
		t.Errorf("user_id should not have been valid: @_appservice_bob::")
	}
}
