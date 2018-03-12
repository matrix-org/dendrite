// Copyright 2017 Vector Creations Ltd
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
	"context"
	"net/http/httptest"
	"testing"

	"encoding/json"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
)

type MockAccountsDB struct{}
type MockDevicesDB struct{}

func (MockAccountsDB) GetAccountByPassword(context.Context, string, string) (*authtypes.Account, error) {
	var fakeAccount = authtypes.Account{
		"reguser:matrix.org",
		"reguser",
		"matrix.org",
		&authtypes.Profile{},
		"",
	}
	return &fakeAccount, nil
}

func (MockDevicesDB) CreateDevice(context.Context, string, *string, string, *string) (*authtypes.Device, error) {
	var fakeDevice = authtypes.Device{
		"fakedevices",
		"requser:matrix.org",
		"asikdhfkhdsafkjsdjkfasdf",
	}
	return &fakeDevice, nil
}

type InvalidAccountDB struct{}
type InvalidDeviceDB struct{}
type fakeErr struct{}

func (fakeErr) Error() string { return "forced error" }

func (InvalidAccountDB) GetAccountByPassword(context.Context, string, string) (*authtypes.Account, error) {
	var err fakeErr
	return nil, err
}

func (InvalidDeviceDB) CreateDevice(context.Context, string, *string, string, *string) (*authtypes.Device, error) {
	var err fakeErr
	return nil, err
}

var (
	accountDB    = MockAccountsDB{}
	deviceDB     = MockDevicesDB{}
	BadAccountDB = InvalidAccountDB{}
	BadDeviceDB  = InvalidDeviceDB{}
	cfg          = config.Dendrite{}
)

type loginRequest struct {
	User     string
	Password string
}

// not sending the user at all
func TestNoUserLogin(t *testing.T) {
	u := loginRequest{"", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, deviceDB, cfg).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_BAD_JSON" {
		t.Error("No usern verification expected error_code 'M_BAD_JSON' got: ", err)
	}
}

// sending invalid username
func TestInvalidUsername(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"@minions", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, deviceDB, cfg).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_INVALID_USERNAME" {
		t.Error("invalid username verification, expected error_code 'M_INVALID_USERNAME' got: ", err)
	}
}

// sending wrong servername in ID
func TestInvalidServerName(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"@minions:notmatrix.org", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, deviceDB, cfg).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_INVALID_USERNAME" {
		t.Error("invalid servername verification, expected error_code 'M_INVALID_USERNAME' got: ", err)
	}
}

// wrong username password
func TestWrongUsernamePassword(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"@minions:matrix.org", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, BadAccountDB, deviceDB, cfg).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_FORBIDDEN" {
		t.Error("wrong username password verification, expected error_code 'M_FORBIDDEN' got: ", err)
	}
}

// devices database failure
func TestDeviceNotCreated(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"minions", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, BadDeviceDB, cfg).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_UNKNOWN" {
		t.Error("device database failure verification, expected error_code 'M_UNKNOWN' got: ", err)
	}
}

// login with userID
func TestValidLoginviaUserID(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"@minions:matrix.org", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, deviceDB, cfg).Code
	if err != 200 {
		t.Error("valid login via userID verification, expected http response code '200' got: ", err)
	}
}

// login with username
func TestValidLoginViaUsername(t *testing.T) {
	cfg.Matrix.ServerName = "matrix.org"
	u := loginRequest{"minions", "blahblah"}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(u)
	req := httptest.NewRequest("POST", "https://api.com", b)
	err := Login(req, accountDB, deviceDB, cfg).Code
	if err != 200 {
		t.Error("valid login verification via username, expected http response code '200' got: ", err)
	}
}
