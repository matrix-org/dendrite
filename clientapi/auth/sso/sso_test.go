package sso

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
)

func TestNewAuthenticator(t *testing.T) {
	_, err := NewAuthenticator(&config.SSO{
		Providers: []config.IdentityProvider{
			{
				Type: config.SSOTypeGitHub,
				OAuth2: config.OAuth2{
					ClientID: "aclientid",
				},
			},
			{
				Type: config.SSOTypeOIDC,
				OIDC: config.OIDC{
					OAuth2: config.OAuth2{
						ClientID: "aclientid",
					},
					DiscoveryURL: "http://oidc.example.com/discovery",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator failed: %v", err)
	}
}

func TestAuthenticator(t *testing.T) {
	ctx := context.Background()

	var idp fakeIdentityProvider
	a := Authenticator{
		providers: map[string]identityProvider{
			"fake": &idp,
		},
	}

	t.Run("authorizationURL", func(t *testing.T) {
		got, err := a.AuthorizationURL(ctx, "fake", "http://matrix.example.com/continue", "anonce")
		if err != nil {
			t.Fatalf("AuthorizationURL failed: %v", err)
		}
		if want := "aurl"; got != want {
			t.Errorf("AuthorizationURL: got %q, want %q", got, want)
		}
	})

	t.Run("processCallback", func(t *testing.T) {
		got, err := a.ProcessCallback(ctx, "fake", "http://matrix.example.com/continue", "anonce", url.Values{})
		if err != nil {
			t.Fatalf("ProcessCallback failed: %v", err)
		}
		if want := (&CallbackResult{DisplayName: "aname"}); !reflect.DeepEqual(got, want) {
			t.Errorf("ProcessCallback: got %+v, want %+v", got, want)
		}
	})
}

type fakeIdentityProvider struct{}

func (idp *fakeIdentityProvider) AuthorizationURL(ctx context.Context, callbackURL, nonce string) (string, error) {
	return "aurl", nil
}

func (idp *fakeIdentityProvider) ProcessCallback(ctx context.Context, callbackURL, nonce string, query url.Values) (*CallbackResult, error) {
	return &CallbackResult{DisplayName: "aname"}, nil
}
