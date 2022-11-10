package sso

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestOIDCIdentityProviderAuthorizationURL(t *testing.T) {
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/discovery", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"authorization_endpoint":"http://oidc.example.com/authorize","token_endpoint":"http://oidc.example.com/token","userinfo_endpoint":"http://oidc.example.com/userinfo","issuer":"http://oidc.example.com/"}`))
	})

	s := httptest.NewServer(mux)
	defer s.Close()

	idp := newOIDCIdentityProvider(&config.IdentityProvider{
		OIDC: config.OIDC{
			OAuth2: config.OAuth2{
				ClientID: "aclientid",
			},
			DiscoveryURL: s.URL + "/discovery",
		},
	}, s.Client())

	got, err := idp.AuthorizationURL(ctx, "https://matrix.example.com/continue", "anonce")
	if err != nil {
		t.Fatalf("AuthorizationURL failed: %v", err)
	}

	if want := "http://oidc.example.com/authorize?client_id=aclientid&redirect_uri=https%3A%2F%2Fmatrix.example.com%2Fcontinue&response_type=code&scope=openid+profile+email&state=anonce"; got != want {
		t.Errorf("AuthorizationURL: got %q, want %q", got, want)
	}
}

func TestOIDCIdentityProviderProcessCallback(t *testing.T) {
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
					Namespace: uapi.OIDCNamespace,
					Issuer:    "http://oidc.example.com/",
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
			var sURL string
			mux.HandleFunc("/discovery", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(fmt.Sprintf(`{"authorization_endpoint":"%s/authorize","token_endpoint":"%s/token","userinfo_endpoint":"%s/userinfo","issuer":"http://oidc.example.com/"}`,
					sURL, sURL, sURL)))
			})
			mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"access_token":"atoken", "token_type":"Bearer"}`))
			})
			mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"sub":"asub", "name":"aname", "preferred_username":"auser"}`))
			})

			s := httptest.NewServer(mux)
			defer s.Close()

			sURL = s.URL
			idp := newOIDCIdentityProvider(&config.IdentityProvider{
				OIDC: config.OIDC{
					OAuth2: config.OAuth2{
						ClientID: "aclientid",
					},
					DiscoveryURL: sURL + "/discovery",
				},
			}, s.Client())

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
