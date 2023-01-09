package internal

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
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
