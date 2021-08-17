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
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
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
	ret := fakeEmptySessions.GetCompletedStages(fakeSessionID)

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
	fakeConfig.Defaults()
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

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	defaults := &config.PasswordRequirements{}
	defaults.Defaults()

	custom := &config.PasswordRequirements{
		MinPasswordLength: 16,
		MaxPasswordLength: 32,
		RequireMixedCase:  true,
		MinNumberSymbols:  5,
	}
	var testCases = []struct {
		name     string
		config   config.PasswordRequirements
		password string
		expected *util.JSONResponse
	}{
		{
			name:     "default reject too short",
			config:   *defaults,
			password: strings.Repeat("a", defaults.MinPasswordLength-1),
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "default reject too long",
			config:   *defaults,
			password: strings.Repeat("a", defaults.MaxPasswordLength+10),
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "default reject empty password",
			config:   *defaults,
			password: "",
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "set min reject too short",
			config:   *custom,
			password: "abcd",
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "set max reject too long",
			config:   *custom,
			password: strings.Repeat("x", custom.MaxPasswordLength+10),
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "set symbols not enough",
			config:   *custom,
			password: "thi$i$apasswordshouldbelong",
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "require mixed case but none given",
			config:   *custom,
			password: "haha_all_lowercase_cant_catch_me",
			expected: &util.JSONResponse{
				Code: http.StatusBadRequest,
			},
		},
		{
			name:     "custom settings allow",
			config:   *custom,
			password: "$0me_$up3r_$trong_P@ass",
			expected: nil,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := validatePassword(test.password, test.config)
			if test.expected == nil && response != nil {
				t.Errorf("expected password to be validated. got error: %s", response.JSON.(*jsonerror.MatrixError).Err)
			} else if test.expected != nil && response == nil {
				t.Errorf("expected password to fail, but was validated")
			} else if test.expected != nil && test.expected.Code != response.Code {
				t.Errorf("expected error code %d, got %d", test.expected.Code, response.Code)
			} else if test.expected != nil && test.expected.Code == response.Code {
				t.Logf("expected error code %d matches %d", test.expected.Code, response.Code)
			} else if test.expected == nil && response == nil {
				t.Log("password validation passed")
			} else {
				t.Errorf("uncaught test result. expected %v, got %v", test.expected, response)
			}
		})
	}
}
