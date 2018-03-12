package routing

import (
	"context"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"net/http/httptest"
	"testing"
)

func (MockAccountsDB) GetProfileByLocalpart(context.Context, string) (*authtypes.Profile, error) {
	return &authtypes.Profile{
		"minions",
		"minions",
		"http://minion.org",
	}, nil
}

func (MockAccountsDB) SetAvatarURL(context.Context, string, string) error {
	return nil
}

func (MockAccountsDB) GetMembershipsByLocalpart(context.Context, string) ([]authtypes.Membership, error) {
	return []authtypes.Membership{
		{
			"minions",
			"despicable_me",
			"mission_1",
		},
		{
			"minions",
			"despicable_me_2",
			"mission_2",
		},
	}, nil
}

func (MockAccountsDB) SetDisplayName(context.Context, string, string) error {
	return nil
}

func (InvalidAccountDB) GetProfileByLocalpart(context.Context, string) (*authtypes.Profile, error) {
	var err fakeErr
	return nil, err
}

func (InvalidAccountDB) SetAvatarURL(context.Context, string, string) error {
	var err fakeErr
	return err
}

func (InvalidAccountDB) GetMembershipsByLocalpart(context.Context, string) ([]authtypes.Membership, error) {
	var err fakeErr
	return nil, err
}

func (InvalidAccountDB) SetDisplayName(context.Context, string, string) error {
	var err fakeErr
	return err
}

// Post method
func TestPostMethodProfile(t *testing.T) {
	const userID = "@minions:matrix.org"
	req := httptest.NewRequest("POST", "https://api.com", nil)
	err := GetProfile(req, accountDB, userID).JSON.(*jsonerror.MatrixError).ErrCode
	if err != "M_NOT_FOUND" {
		t.Error("POST method verification expected error_code 'M_NOT_FOUND' got: ", err)
	}
}

// invalid userID
func TestInvalidUserID(t *testing.T) {
	const userID = "@minimatrix"
	req := httptest.NewRequest("GET", "https://api.com", nil)
	err := GetProfile(req, accountDB, userID).Code
	if err != 500 {
		t.Error("invalid userID verification expected error_code '500' got: ", err)
	}
}

// Database Error
func TestDatabaseError(t *testing.T) {
	const userID = "@minions:matrix.org"
	req := httptest.NewRequest("GET", "https://api.com", nil)
	err := GetProfile(req, BadAccountDB, userID).Code
	if err != 500 {
		t.Error("Database Error verification expected error_code '500' got: ", err)
	}
}

// Valid Request
func TestValidGetProfileRequest(t *testing.T) {
	const userID = "@minions:matrix.org"
	req := httptest.NewRequest("GET", "https://api.com", nil)
	err := GetProfile(req, accountDB, userID).Code
	if err != 200 {
		t.Error("Valid GetProfile verification expected code '200' got: ", err)
	}
}

// Todo write similar tests for other get and put functions
