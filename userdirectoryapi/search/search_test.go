package search

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	searchtypes "github.com/matrix-org/dendrite/userdirectoryapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type MockAccountDatabase struct {
	limited   bool
	wantError bool
}

func (d *MockAccountDatabase) GetAccountByPassword(
	ctx context.Context, localpart, plaintextPassword string,
) (*authtypes.Account, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) SetAvatarURL(
	ctx context.Context, localpart string, avatarURL string,
) error {
	panic("implement me")
}

func (d *MockAccountDatabase) SetDisplayName(
	ctx context.Context, localpart string, displayName string,
) error {
	panic("implement me")
}

func (d *MockAccountDatabase) CreateAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string,
) (*authtypes.Account, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) UpdateMemberships(
	ctx context.Context, eventsToAdd []gomatrixserverlib.Event, idsToRemove []string,
) error {
	panic("implement me")
}

func (d *MockAccountDatabase) GetMembershipInRoomByLocalpart(
	ctx context.Context, localpart, roomID string,
) (authtypes.Membership, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetMembershipsByLocalpart(
	ctx context.Context, localpart string,
) (memberships []authtypes.Membership, err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) SaveAccountData(
	ctx context.Context, localpart, roomID, dataType, content string,
) error {
	panic("implement me")
}

func (d *MockAccountDatabase) GetAccountData(ctx context.Context, localpart string) (
	global []gomatrixserverlib.ClientEvent,
	rooms map[string][]gomatrixserverlib.ClientEvent,
	err error,
) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data *gomatrixserverlib.ClientEvent, err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetNewNumericLocalpart(
	ctx context.Context,
) (int64, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) SaveThreePIDAssociation(
	ctx context.Context, threepid, localpart, medium string,
) (err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) RemoveThreePIDAssociation(
	ctx context.Context, threepid string, medium string,
) (err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetLocalpartForThreePID(
	ctx context.Context, threepid string, medium string,
) (localpart string, err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetFilter(
	ctx context.Context, localpart string, filterID string,
) (*gomatrixserverlib.Filter, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) PutFilter(
	ctx context.Context, localpart string, filter *gomatrixserverlib.Filter,
) (string, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) CheckAccountAvailability(ctx context.Context, localpart string) (bool, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) GetAccountByLocalpart(ctx context.Context, localpart string,
) (*authtypes.Account, error) {
	panic("implement me")
}

func (d *MockAccountDatabase) SearchUserIdAndDisplayName(ctx context.Context, searchTerm string, limit int8) (*[]searchtypes.SearchResult, bool, error) {
	var searchResults []searchtypes.SearchResult
	searchResults = append(searchResults, searchtypes.SearchResult{UserId: "alice:localhost", AvatarUrl: "", DisplayName: ""})
	if d.wantError {
		return nil, false, fmt.Errorf("")
	} else {
		return &searchResults, d.limited, nil
	}
}

func TestSearch(t *testing.T) {
	type testCase struct {
		name        string
		wantError   bool
		errorCode   int
		results     []string
		limited     bool
		requestBody string
	}
	tt := []testCase{
		{
			"Find user do not hit limit",
			false,
			0,
			[]string{"alice:localhost"},
			false,
			"{ \"search_term\": \"alice\" }",
		},
		{
			"Find user do hit limit",
			false,
			0,
			[]string{"alice:localhost"},
			true,
			"{ \"search_term\": \"alice\" }",
		},
		{
			"Find user and fail",
			true,
			500,
			[]string{"alice:localhost"},
			false,
			"{ \"search_term\": \"alice\" }",
		},
		{
			"Find user and fail to parse request",
			true,
			400,
			[]string{"alice:localhost"},
			false,
			"{ \"search_term\": \"alicINVALID }",
		},
	}

	setupAccountDb := func(limited bool, wantError bool) accounts.AccountDatabase {
		return &MockAccountDatabase{limited, wantError}
	}

	for _, tc := range tt {
		fmt.Printf("Executing %s", tc.name)
		requestBody := ioutil.NopCloser(strings.NewReader(tc.requestBody))
		req := &http.Request{Body: requestBody}
		searchResults := Search(req, setupAccountDb(tc.limited, tc.wantError))
		if tc.wantError {
			assert.EqualValues(t, searchResults.Code, tc.errorCode)
		} else {
			response := searchResults.JSON.(userSearchResponse)
			for _, result := range tc.results {
				exists := false
				for _, item := range *response.Results {
					if item.UserId == result {
						exists = true
						break
					}
				}
				assert.EqualValues(t, true, exists, "Expected value not found in response")
			}
		}
	}

}
