package sso

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestOAuth2IdentityProviderAuthorizationURL(t *testing.T) {
	ctx := context.Background()

	idp := &oauth2IdentityProvider{
		cfg: &config.IdentityProvider{
			OAuth2: config.OAuth2{
				ClientID: "aclientid",
			},
		},
		hc: http.DefaultClient,

		authorizationURL: "https://oauth2.example.com/authorize",
	}
	idp.oauth2Cfg = &idp.cfg.OAuth2

	got, err := idp.AuthorizationURL(ctx, "https://matrix.example.com/continue", "anonce")
	if err != nil {
		t.Fatalf("AuthorizationURL failed: %v", err)
	}

	if want := "https://oauth2.example.com/authorize?client_id=aclientid&redirect_uri=https%3A%2F%2Fmatrix.example.com%2Fcontinue&response_type=code&scope=&state=anonce"; got != want {
		t.Errorf("AuthorizationURL: got %q, want %q", got, want)
	}
}

func TestOAuth2IdentityProviderProcessCallback(t *testing.T) {
	ctx := context.Background()

	const callbackURL = "https://matrix.example.com/continue"

	tsts := []struct {
		Name  string
		Query url.Values

		Want         *CallbackResult
		WantTokenReq url.Values
	}{
		{
			Name: "gotEverything",
			Query: url.Values{
				"code":  []string{"acode"},
				"state": []string{"anonce"},
			},

			Want: &CallbackResult{
				Identifier: &UserIdentifier{
					Namespace: uapi.SSOIDNamespace,
					Issuer:    "anid",
					Subject:   "asub",
				},
				DisplayName:     "aname",
				SuggestedUserID: "auser",
			},
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"access_token":"atoken", "token_type":"Bearer"}`))
			})
			mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"sub":"asub", "name":"aname", "preferred_user":"auser"}`))
			})

			s := httptest.NewServer(mux)
			defer s.Close()

			idp := &oauth2IdentityProvider{
				cfg: &config.IdentityProvider{
					ID: "anid",
					OAuth2: config.OAuth2{
						ClientID:     "aclientid",
						ClientSecret: "aclientsecret",
					},
				},
				hc: s.Client(),

				accessTokenURL: s.URL + "/token",
				userInfoURL:    s.URL + "/userinfo",

				subPath:             "sub",
				displayNamePath:     "name",
				suggestedUserIDPath: "preferred_user",
			}
			idp.oauth2Cfg = &idp.cfg.OAuth2

			got, err := idp.ProcessCallback(ctx, callbackURL, "anonce", tst.Query)
			if err != nil {
				t.Fatalf("ProcessCallback failed: %v", err)
			}

			if !reflect.DeepEqual(got, tst.Want) {
				t.Errorf("ProcessCallback: got %+v, want %+v", got, tst.Want)
			}
		})
	}
}

func TestOAuth2IdentityProviderGetAccessToken(t *testing.T) {
	ctx := context.Background()

	const callbackURL = "https://matrix.example.com/continue"

	mux := http.NewServeMux()
	var gotReq url.Values
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		gotReq = r.Form

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"atoken", "token_type":"Bearer"}`))
	})

	s := httptest.NewServer(mux)
	defer s.Close()

	idp := &oauth2IdentityProvider{
		cfg: &config.IdentityProvider{
			ID: "anid",
			OAuth2: config.OAuth2{
				ClientID:     "aclientid",
				ClientSecret: "aclientsecret",
			},
		},
		hc: s.Client(),

		accessTokenURL: s.URL + "/token",
	}
	idp.oauth2Cfg = &idp.cfg.OAuth2

	got, err := idp.getAccessToken(ctx, callbackURL, "acode")
	if err != nil {
		t.Fatalf("getAccessToken failed: %v", err)
	}

	if want := "atoken"; got != want {
		t.Errorf("getAccessToken: got %q, want %q", got, want)
	}

	wantReq := url.Values{
		"client_id":     []string{"aclientid"},
		"client_secret": []string{"aclientsecret"},
		"code":          []string{"acode"},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{callbackURL},
	}
	if !reflect.DeepEqual(gotReq, wantReq) {
		t.Errorf("getAccessToken request: got %+v, want %+v", gotReq, wantReq)
	}
}

func TestOAuth2IdentityProviderGetUserInfo(t *testing.T) {
	ctx := context.Background()

	mux := http.NewServeMux()
	var gotHeader http.Header
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sub":"asub", "name":"aname", "preferred_user":"auser"}`))
	})

	s := httptest.NewServer(mux)
	defer s.Close()

	idp := &oauth2IdentityProvider{
		cfg: &config.IdentityProvider{
			ID: "anid",
			OAuth2: config.OAuth2{
				ClientID:     "aclientid",
				ClientSecret: "aclientsecret",
			},
		},
		hc: s.Client(),

		userInfoURL: s.URL + "/userinfo",

		responseMimeType:    "application/json",
		subPath:             "sub",
		displayNamePath:     "name",
		suggestedUserIDPath: "preferred_user",
	}
	idp.oauth2Cfg = &idp.cfg.OAuth2

	gotSub, gotName, gotSuggestedUser, err := idp.getUserInfo(ctx, "atoken")
	if err != nil {
		t.Fatalf("getUserInfo failed: %v", err)
	}

	if want := "asub"; gotSub != want {
		t.Errorf("getUserInfo subject: got %q, want %q", gotSub, want)
	}
	if want := "aname"; gotName != want {
		t.Errorf("getUserInfo displayName: got %q, want %q", gotName, want)
	}
	if want := "auser"; gotSuggestedUser != want {
		t.Errorf("getUserInfo suggestedUser: got %q, want %q", gotSuggestedUser, want)
	}

	gotHeader.Del("Accept-Encoding")
	gotHeader.Del("User-Agent")
	wantHeader := http.Header{
		"Accept":        []string{"application/json"},
		"Authorization": []string{"Bearer atoken"},
	}
	if !reflect.DeepEqual(gotHeader, wantHeader) {
		t.Errorf("getUserInfo header: got %+v, want %+v", gotHeader, wantHeader)
	}
}
