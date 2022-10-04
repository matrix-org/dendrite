package routing

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matrix-org/dendrite/clientapi/auth/sso"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestSSORedirect(t *testing.T) {
	tsts := []struct {
		Name   string
		Req    http.Request
		IDPID  string
		Auth   fakeSSOAuthenticator
		Config config.SSO

		WantLocationRE  string
		WantSetCookieRE string
	}{
		{
			Name: "redirectDefault",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{
						"redirectUrl": []string{"http://example.com/continue"},
					}.Encode(),
				},
			},
			Config: config.SSO{
				DefaultProviderID: "adefault",
			},
			WantLocationRE:  `http://auth.example.com/authorize\?callbackURL=http%3A%2F%2Fmatrix.example.com%2F_matrix%2Fv4%2Flogin%2Fsso%2Fcallback%3Fprovider%3Dadefault&nonce=.+&providerID=adefault`,
			WantSetCookieRE: "sso_nonce=[^;].*Path=/_matrix/v4/login/sso",
		},
		{
			Name: "redirectFirstProvider",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{
						"redirectUrl": []string{"http://example.com/continue"},
					}.Encode(),
				},
			},
			Config: config.SSO{
				Providers: []config.IdentityProvider{
					{ID: "firstprovider"},
					{ID: "secondprovider"},
				},
			},
			WantLocationRE:  `http://auth.example.com/authorize\?callbackURL=http%3A%2F%2Fmatrix.example.com%2F_matrix%2Fv4%2Flogin%2Fsso%2Fcallback%3Fprovider%3Dfirstprovider&nonce=.+&providerID=firstprovider`,
			WantSetCookieRE: "sso_nonce=[^;].*Path=/_matrix/v4/login/sso",
		},
		{
			Name: "redirectExplicitProvider",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{
						"redirectUrl": []string{"http://example.com/continue"},
					}.Encode(),
				},
			},
			IDPID:           "someprovider",
			WantLocationRE:  `http://auth.example.com/authorize\?callbackURL=http.*%3Fprovider%3Dsomeprovider&nonce=.+&providerID=someprovider`,
			WantSetCookieRE: "sso_nonce=[^;].*Path=/_matrix/v4/login/sso",
		},
		{
			Name: "redirectEmptyredirectPath",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{
						"redirectUrl": []string{"http://example.com"},
					}.Encode(),
				},
			},
			IDPID:           "someprovider",
			WantLocationRE:  `http://auth.example.com/authorize\?callbackURL=http.*%3Fprovider%3Dsomeprovider&nonce=.+&providerID=someprovider`,
			WantSetCookieRE: "sso_nonce=[^;].*Path=/_matrix/v4/login/sso",
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got := SSORedirect(&tst.Req, tst.IDPID, &tst.Auth, &tst.Config)

			if want := http.StatusFound; got.Code != want {
				t.Errorf("SSORedirect Code: got %v, want %v", got.Code, want)
			}

			if m, err := regexp.MatchString(tst.WantLocationRE, got.Headers["Location"]); err != nil {
				t.Fatalf("WantSetCookieRE failed: %v", err)
			} else if !m {
				t.Errorf("SSORedirect Location: got %q, want match %v", got.Headers["Location"], tst.WantLocationRE)
			}

			if m, err := regexp.MatchString(tst.WantSetCookieRE, got.Headers["Set-Cookie"]); err != nil {
				t.Fatalf("WantSetCookieRE failed: %v", err)
			} else if !m {
				t.Errorf("SSORedirect Set-Cookie: got %q, want match %v", got.Headers["Set-Cookie"], tst.WantSetCookieRE)
			}
		})
	}
}

func TestSSORedirectError(t *testing.T) {
	tsts := []struct {
		Name   string
		Req    http.Request
		IDPID  string
		Auth   fakeSSOAuthenticator
		Config config.SSO

		WantCode int
	}{
		{
			Name: "missingRedirectUrl",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path:     "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{}.Encode(),
				},
			},
			WantCode: http.StatusBadRequest,
		},
		{
			Name: "invalidRedirectUrl",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/redirect",
					RawQuery: url.Values{
						"redirectUrl": []string{"/continue"},
					}.Encode(),
				},
			},
			WantCode: http.StatusBadRequest,
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got := SSORedirect(&tst.Req, tst.IDPID, &tst.Auth, &tst.Config)

			if got.Code != tst.WantCode {
				t.Errorf("SSORedirect Code: got %v, want %v", got.Code, tst.WantCode)
			}
		})
	}
}

func TestSSOCallback(t *testing.T) {
	nonce := "1234." + base64.RawURLEncoding.EncodeToString([]byte("http://matrix.example.com/continue"))

	tsts := []struct {
		Name    string
		Req     http.Request
		UserAPI fakeUserAPIForSSO
		Auth    fakeSSOAuthenticator
		Config  config.SSO

		WantLocationRE  string
		WantSetCookieRE string

		WantAccountCreation    []*uapi.PerformAccountCreationRequest
		WantLoginTokenCreation []*uapi.PerformLoginTokenCreationRequest
		WantSaveSSOAssociation []*uapi.PerformSaveSSOAssociationRequest
		WantQueryLocalpart     []*uapi.QueryLocalpartForSSORequest
	}{
		{
			Name: "logIn",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: nonce,
					}).String()},
				},
			},
			UserAPI: fakeUserAPIForSSO{
				localpart: "alocalpart",
			},
			Auth: fakeSSOAuthenticator{
				callbackResult: sso.CallbackResult{
					Identifier: &sso.UserIdentifier{
						Namespace: "anamespace",
						Issuer:    "anissuer",
						Subject:   "asubject",
					},
				},
			},
			WantLocationRE:  `http://matrix.example.com/continue\?loginToken=atoken`,
			WantSetCookieRE: "sso_nonce=;",

			WantLoginTokenCreation: []*uapi.PerformLoginTokenCreationRequest{{Data: uapi.LoginTokenData{UserID: "@alocalpart:aservername"}}},
			WantQueryLocalpart:     []*uapi.QueryLocalpartForSSORequest{{Namespace: "anamespace", Issuer: "anissuer", Subject: "asubject"}},
		},
		{
			Name: "registerSuggested",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: nonce,
					}).String()},
				},
			},
			Auth: fakeSSOAuthenticator{
				callbackResult: sso.CallbackResult{
					Identifier: &sso.UserIdentifier{
						Namespace: "anamespace",
						Issuer:    "anissuer",
						Subject:   "asubject",
					},
					SuggestedUserID: "asuggestedid",
				},
			},
			WantLocationRE:  `http://matrix.example.com/continue\?loginToken=atoken`,
			WantSetCookieRE: "sso_nonce=;",

			WantAccountCreation:    []*uapi.PerformAccountCreationRequest{{Localpart: "asuggestedid", AccountType: uapi.AccountTypeUser, OnConflict: uapi.ConflictAbort}},
			WantLoginTokenCreation: []*uapi.PerformLoginTokenCreationRequest{{Data: uapi.LoginTokenData{UserID: "@asuggestedid:aservername"}}},
			WantSaveSSOAssociation: []*uapi.PerformSaveSSOAssociationRequest{{Namespace: "anamespace", Issuer: "anissuer", Subject: "asubject", Localpart: "asuggestedid"}},
			WantQueryLocalpart:     []*uapi.QueryLocalpartForSSORequest{{Namespace: "anamespace", Issuer: "anissuer", Subject: "asubject"}},
		},
		{
			Name: "registerNumeric",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: nonce,
					}).String()},
				},
			},
			Auth: fakeSSOAuthenticator{
				callbackResult: sso.CallbackResult{
					Identifier: &sso.UserIdentifier{
						Namespace: "anamespace",
						Issuer:    "anissuer",
						Subject:   "asubject",
					},
				},
			},
			WantLocationRE:  `http://matrix.example.com/continue\?loginToken=atoken`,
			WantSetCookieRE: "sso_nonce=;",

			WantAccountCreation:    []*uapi.PerformAccountCreationRequest{{Localpart: "12345", AccountType: uapi.AccountTypeUser, OnConflict: uapi.ConflictAbort}},
			WantLoginTokenCreation: []*uapi.PerformLoginTokenCreationRequest{{Data: uapi.LoginTokenData{UserID: "@12345:aservername"}}},
			WantSaveSSOAssociation: []*uapi.PerformSaveSSOAssociationRequest{{Namespace: "anamespace", Issuer: "anissuer", Subject: "asubject", Localpart: "12345"}},
			WantQueryLocalpart:     []*uapi.QueryLocalpartForSSORequest{{Namespace: "anamespace", Issuer: "anissuer", Subject: "asubject"}},
		},
		{
			Name: "noIdentifierRedirectURL",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: nonce,
					}).String()},
				},
			},
			Auth: fakeSSOAuthenticator{
				callbackResult: sso.CallbackResult{
					RedirectURL: "http://auth.example.com/notdone",
				},
			},
			WantLocationRE:  `http://auth.example.com/notdone`,
			WantSetCookieRE: "^$",
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got := SSOCallback(&tst.Req, &tst.UserAPI, &tst.Auth, &tst.Config, "aservername")

			if want := http.StatusFound; got.Code != want {
				t.Log(got)
				t.Errorf("SSOCallback Code: got %v, want %v", got.Code, want)
			}

			if m, err := regexp.MatchString(tst.WantLocationRE, got.Headers["Location"]); err != nil {
				t.Fatalf("WantSetCookieRE failed: %v", err)
			} else if !m {
				t.Errorf("SSOCallback Location: got %q, want match %v", got.Headers["Location"], tst.WantLocationRE)
			}

			if m, err := regexp.MatchString(tst.WantSetCookieRE, got.Headers["Set-Cookie"]); err != nil {
				t.Fatalf("WantSetCookieRE failed: %v", err)
			} else if !m {
				t.Errorf("SSOCallback Set-Cookie: got %q, want match %v", got.Headers["Set-Cookie"], tst.WantSetCookieRE)
			}

			if diff := cmp.Diff(tst.WantAccountCreation, tst.UserAPI.gotAccountCreation); diff != "" {
				t.Errorf("PerformAccountCreation: +got -want:\n%s", diff)
			}
			if diff := cmp.Diff(tst.WantLoginTokenCreation, tst.UserAPI.gotLoginTokenCreation); diff != "" {
				t.Errorf("PerformLoginTokenCreation: +got -want:\n%s", diff)
			}
			if diff := cmp.Diff(tst.WantSaveSSOAssociation, tst.UserAPI.gotSaveSSOAssociation); diff != "" {
				t.Errorf("PerformSaveSSOAssociation: +got -want:\n%s", diff)
			}
			if diff := cmp.Diff(tst.WantQueryLocalpart, tst.UserAPI.gotQueryLocalpart); diff != "" {
				t.Errorf("QueryLocalpartForSSO: +got -want:\n%s", diff)
			}
		})
	}
}

func TestSSOCallbackError(t *testing.T) {
	nonce := "1234." + base64.RawURLEncoding.EncodeToString([]byte("http://matrix.example.com/continue"))
	goodReq := http.Request{
		Host: "matrix.example.com",
		URL: &url.URL{
			Path: "/_matrix/v4/login/sso/callback",
			RawQuery: url.Values{
				"provider": []string{"aprovider"},
			}.Encode(),
		},
		Header: http.Header{
			"Cookie": []string{(&http.Cookie{
				Name:  "sso_nonce",
				Value: nonce,
			}).String()},
		},
	}
	goodAuth := fakeSSOAuthenticator{
		callbackResult: sso.CallbackResult{
			Identifier: &sso.UserIdentifier{
				Namespace: "anamespace",
				Issuer:    "anissuer",
				Subject:   "asubject",
			},
		},
	}
	errMocked := errors.New("mocked error")

	tsts := []struct {
		Name    string
		Req     http.Request
		UserAPI fakeUserAPIForSSO
		Auth    fakeSSOAuthenticator
		Config  config.SSO

		WantCode int
	}{
		{
			Name: "missingProvider",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: nonce,
					}).String()},
				},
			},
			WantCode: http.StatusBadRequest,
		},
		{
			Name: "missingCookie",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
			},
			WantCode: http.StatusBadRequest,
		},
		{
			Name: "malformedCookie",
			Req: http.Request{
				Host: "matrix.example.com",
				URL: &url.URL{
					Path: "/_matrix/v4/login/sso/callback",
					RawQuery: url.Values{
						"provider": []string{"aprovider"},
					}.Encode(),
				},
				Header: http.Header{
					"Cookie": []string{(&http.Cookie{
						Name:  "sso_nonce",
						Value: "badvalue",
					}).String()},
				},
			},
			WantCode: http.StatusBadRequest,
		},
		{
			Name: "failedProcessCallback",
			Req:  goodReq,
			Auth: fakeSSOAuthenticator{
				callbackErr: errMocked,
			},
			WantCode: http.StatusInternalServerError,
		},
		{
			Name: "failedQueryLocalpartForSSO",
			Req:  goodReq,
			UserAPI: fakeUserAPIForSSO{
				localpartErr: errMocked,
			},
			Auth:     goodAuth,
			WantCode: http.StatusUnauthorized,
		},
		{
			Name: "failedQueryNumericLocalpart",
			Req:  goodReq,
			UserAPI: fakeUserAPIForSSO{
				numericLocalpartErr: errMocked,
			},
			Auth:     goodAuth,
			WantCode: http.StatusInternalServerError,
		},
		{
			Name: "failedAccountCreation",
			Req:  goodReq,
			UserAPI: fakeUserAPIForSSO{
				accountCreationErr: errMocked,
			},
			Auth:     goodAuth,
			WantCode: http.StatusInternalServerError,
		},
		{
			Name: "failedSaveSSOAssociation",
			Req:  goodReq,
			UserAPI: fakeUserAPIForSSO{
				saveSSOAssociationErr: errMocked,
			},
			Auth:     goodAuth,
			WantCode: http.StatusInternalServerError,
		},
		{
			Name: "failedPerformLoginTokenCreation",
			Req:  goodReq,
			UserAPI: fakeUserAPIForSSO{
				localpart:             "alocalpart",
				tokenTokenCreationErr: errMocked,
			},
			Auth:     goodAuth,
			WantCode: http.StatusInternalServerError,
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			got := SSOCallback(&tst.Req, &tst.UserAPI, &tst.Auth, &tst.Config, "aservername")

			if got.Code != tst.WantCode {
				t.Log(got)
				t.Errorf("SSOCallback Code: got %v, want %v", got.Code, tst.WantCode)
			}
		})
	}
}

type fakeSSOAuthenticator struct {
	callbackResult sso.CallbackResult
	callbackErr    error
}

func (auth *fakeSSOAuthenticator) AuthorizationURL(ctx context.Context, providerID, callbackURL, nonce string) (string, error) {
	if providerID == "" {
		return "", errors.New("empty providerID")
	}

	return (&url.URL{
		Scheme: "http",
		Host:   "auth.example.com",
		Path:   "/authorize",
	}).ResolveReference(&url.URL{
		RawQuery: url.Values{
			"callbackURL": []string{callbackURL},
			"nonce":       []string{nonce},
			"providerID":  []string{providerID},
		}.Encode(),
	}).String(), nil
}

func (auth *fakeSSOAuthenticator) ProcessCallback(ctx context.Context, providerID, callbackURL, nonce string, query url.Values) (*sso.CallbackResult, error) {
	return &auth.callbackResult, auth.callbackErr
}

type fakeUserAPIForSSO struct {
	userAPIForSSO

	accountCreationErr    error
	tokenTokenCreationErr error
	saveSSOAssociationErr error
	localpart             string
	localpartErr          error
	numericLocalpartErr   error

	gotAccountCreation    []*uapi.PerformAccountCreationRequest
	gotLoginTokenCreation []*uapi.PerformLoginTokenCreationRequest
	gotSaveSSOAssociation []*uapi.PerformSaveSSOAssociationRequest
	gotQueryLocalpart     []*uapi.QueryLocalpartForSSORequest
}

func (userAPI *fakeUserAPIForSSO) PerformAccountCreation(ctx context.Context, req *uapi.PerformAccountCreationRequest, res *uapi.PerformAccountCreationResponse) error {
	userAPI.gotAccountCreation = append(userAPI.gotAccountCreation, req)
	return userAPI.accountCreationErr
}

func (userAPI *fakeUserAPIForSSO) PerformLoginTokenCreation(ctx context.Context, req *uapi.PerformLoginTokenCreationRequest, res *uapi.PerformLoginTokenCreationResponse) error {
	userAPI.gotLoginTokenCreation = append(userAPI.gotLoginTokenCreation, req)
	res.Metadata = uapi.LoginTokenMetadata{
		Token: "atoken",
	}
	return userAPI.tokenTokenCreationErr
}

func (userAPI *fakeUserAPIForSSO) PerformSaveSSOAssociation(ctx context.Context, req *uapi.PerformSaveSSOAssociationRequest, res *struct{}) error {
	userAPI.gotSaveSSOAssociation = append(userAPI.gotSaveSSOAssociation, req)
	return userAPI.saveSSOAssociationErr
}

func (userAPI *fakeUserAPIForSSO) QueryLocalpartForSSO(ctx context.Context, req *uapi.QueryLocalpartForSSORequest, res *uapi.QueryLocalpartForSSOResponse) error {
	userAPI.gotQueryLocalpart = append(userAPI.gotQueryLocalpart, req)
	res.Localpart = userAPI.localpart
	return userAPI.localpartErr
}

func (userAPI *fakeUserAPIForSSO) QueryNumericLocalpart(ctx context.Context, res *uapi.QueryNumericLocalpartResponse) error {
	res.ID = 12345
	return userAPI.numericLocalpartErr
}
