package userapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/matrix-org/dendrite/userapi/mail"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matryer/is"
	"golang.org/x/crypto/bcrypt"
)

const (
	serverName = gomatrixserverlib.ServerName("example.com")
)

var (
	testReq = &api.CreateSessionRequest{
		ClientSecret: "foobar",
		NextLink:     "http://foobar.com",
		ThreePid:     "foo@bar.com",
		Extra:        []string{},
		SendAttempt:  0,
	}
	ctx    = context.Background()
	mailer = &testMailer{
		c: []chan *mail.Mail{
			api.AccountPassword: make(chan *mail.Mail, 3),
			api.AccountThreepid: make(chan *mail.Mail, 3),
			api.Register:        make(chan *mail.Mail, 3),
		},
	}
)

type testMailer struct {
	c []chan *mail.Mail
}

func (tm *testMailer) Send(s *mail.Mail, t api.ThreepidSessionType) error {
	tm.c[t] <- s
	return nil
}

func mustMakeInternalAPI(is *is.I) (*internal.UserInternalAPI, accounts.Database) {
	accountDB, err := accounts.NewDatabase(&config.DatabaseOptions{
		ConnectionString: "file::memory:",
	}, serverName, bcrypt.MinCost, config.DefaultOpenIDTokenLifetimeMS)
	is.NoErr(err)
	cfg := &config.UserAPI{
		DeviceDatabase: config.DatabaseOptions{
			ConnectionString:   "file::memory:",
			MaxOpenConnections: 1,
			MaxIdleConnections: 1,
		},
		Matrix: &config.Global{
			ServerName: serverName,
		},
		ThreepidDatabase: config.DatabaseOptions{
			ConnectionString:   "file::memory:",
			MaxOpenConnections: 1,
			MaxIdleConnections: 1,
		},
		Email: config.EmailConf{
			TemplatesPath: "../res/default",
		},
	}
	apiInt := userapi.NewInternalAPI(accountDB, cfg, nil, nil).(*internal.UserInternalAPI)
	return apiInt, accountDB
}

func TestQueryProfile(t *testing.T) {
	is := is.New(t)
	aliceAvatarURL := "mxc://example.com/alice"
	aliceDisplayName := "Alice"
	userAPI, accountDB := mustMakeInternalAPI(is)
	_, err := accountDB.CreateAccount(context.TODO(), "alice", "foobar", "")
	if err != nil {
		t.Fatalf("failed to make account: %s", err)
	}
	if err := accountDB.SetAvatarURL(context.TODO(), "alice", aliceAvatarURL); err != nil {
		t.Fatalf("failed to set avatar url: %s", err)
	}
	if err := accountDB.SetDisplayName(context.TODO(), "alice", aliceDisplayName); err != nil {
		t.Fatalf("failed to set display name: %s", err)
	}

	testCases := []struct {
		req     api.QueryProfileRequest
		wantRes api.QueryProfileResponse
		wantErr error
	}{
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@alice:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists:  true,
				AvatarURL:   aliceAvatarURL,
				DisplayName: aliceDisplayName,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@bob:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists: false,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: "@alice:wrongdomain.com",
			},
			wantErr: fmt.Errorf("wrong domain"),
		},
	}

	runCases := func(testAPI api.UserInternalAPI) {
		for _, tc := range testCases {
			var gotRes api.QueryProfileResponse
			gotErr := testAPI.QueryProfile(context.TODO(), &tc.req, &gotRes)
			if tc.wantErr == nil && gotErr != nil || tc.wantErr != nil && gotErr == nil {
				t.Errorf("QueryProfile error, got %s want %s", gotErr, tc.wantErr)
				continue
			}
			if !reflect.DeepEqual(tc.wantRes, gotRes) {
				t.Errorf("QueryProfile response got %+v want %+v", gotRes, tc.wantRes)
			}
		}
	}

	t.Run("HTTP API", func(t *testing.T) {
		router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
		userapi.AddInternalRoutes(router, userAPI)
		apiURL, cancel := test.ListenAndServe(t, router, false)
		defer cancel()
		httpAPI, err := inthttp.NewUserAPIClient(apiURL, &http.Client{})
		if err != nil {
			t.Fatalf("failed to create HTTP client")
		}
		runCases(httpAPI)
	})
	t.Run("Monolith", func(t *testing.T) {
		runCases(userAPI)
	})
}

func TestCreateSession(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	mustCreateSession(is, internalApi)
}

func TestCreateSession_Twice(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	mustCreateSession(is, internalApi)
	resp := api.CreateSessionResponse{}
	err := internalApi.CreateSession(ctx, testReq, &resp)
	is.NoErr(err)
	is.Equal(resp.Sid, int64(1))
	select {
	case <-mailer.c[api.AccountPassword]:
		t.Fatal("email was received, but sent attempt was not increased")
	default:
		break
	}
}

func TestCreateSession_Twice_IncreaseSendAttempt(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	mustCreateSession(is, internalApi)
	resp := api.CreateSessionResponse{}
	testReqBumped := *testReq
	testReqBumped.SendAttempt = 1
	err := internalApi.CreateSession(ctx, &testReqBumped, &resp)
	is.NoErr(err)
	is.Equal(resp.Sid, int64(1))
	sub := <-mailer.c[api.AccountPassword]
	is.Equal(len(sub.Token), 64)
	is.Equal(sub.To, testReq.ThreePid)
}

func TestValidateSession(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	s, token := mustCreateSession(is, internalApi)
	mustValidateSesson(is, internalApi, testReq.ClientSecret, token, s.Sid)
}

func TestIsSessionValidated_InvalidatedSession(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	s, _ := mustCreateSession(is, internalApi)
	resp := api.IsSessionValidatedResponse{}
	err := internalApi.IsSessionValidated(ctx, &api.SessionOwnership{
		Sid:          s.Sid,
		ClientSecret: testReq.ClientSecret,
	}, &resp)
	is.NoErr(err)
	is.Equal(resp.Validated, false)
}

func TestIsSessionValidated_ValidatedSession(t *testing.T) {
	is := is.New(t)
	internalApi, _ := mustMakeInternalAPI(is)
	s, token := mustCreateSession(is, internalApi)
	resp := api.IsSessionValidatedResponse{}
	mustValidateSesson(is, internalApi, testReq.ClientSecret, token, s.Sid)
	err := internalApi.IsSessionValidated(ctx, &api.SessionOwnership{
		Sid:          s.Sid,
		ClientSecret: testReq.ClientSecret,
	}, &resp)
	is.NoErr(err)
	is.Equal(resp.Validated, true)
	is.Equal(resp.ValidatedAt > 0, true)
}

func mustCreateSession(is *is.I, i *internal.UserInternalAPI) (resp *api.CreateSessionResponse, token string) {
	resp = &api.CreateSessionResponse{}
	i.Mail = mailer
	err := i.CreateSession(ctx, testReq, resp)
	is.NoErr(err)
	is.Equal(resp.Sid, int64(1))
	sub := <-mailer.c[api.AccountPassword]
	is.Equal(len(sub.Token), 64)
	is.Equal(sub.To, testReq.ThreePid)
	submitUrl, err := url.Parse(sub.Link)
	is.NoErr(err)
	is.Equal(submitUrl.Host, "example.com")
	is.Equal(submitUrl.Path, "/_matrix/client/r0/account/password/email/submitToken")
	q := submitUrl.Query()
	is.Equal(q["sid"][0], "1")
	is.Equal(q["token"][0], sub.Token)
	is.Equal(q["client_secret"][0], "foobar")
	token = sub.Token
	return
}

func mustValidateSesson(is *is.I, i *internal.UserInternalAPI, secret, token string, sid int64) {
	err := i.ValidateSession(ctx, &api.ValidateSessionRequest{
		SessionOwnership: api.SessionOwnership{
			Sid:          sid,
			ClientSecret: secret,
		},
		Token: token,
	},
		struct{}{},
	)
	is.NoErr(err)
}
