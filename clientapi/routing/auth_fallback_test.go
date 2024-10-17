package routing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/setup/config"
)

func Test_AuthFallback(t *testing.T) {
	cfg := config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{Generate: true, SingleDatabase: true})
	for _, useHCaptcha := range []bool{false, true} {
		for _, recaptchaEnabled := range []bool{false, true} {
			for _, wantErr := range []bool{false, true} {
				t.Run(fmt.Sprintf("useHCaptcha(%v) - recaptchaEnabled(%v) - wantErr(%v)", useHCaptcha, recaptchaEnabled, wantErr), func(t *testing.T) {
					// Set the defaults for each test
					cfg.ClientAPI.Defaults(config.DefaultOpts{Generate: true, SingleDatabase: true})
					cfg.ClientAPI.RecaptchaEnabled = recaptchaEnabled
					cfg.ClientAPI.RecaptchaPublicKey = "pub"
					cfg.ClientAPI.RecaptchaPrivateKey = "priv"
					if useHCaptcha {
						cfg.ClientAPI.RecaptchaSiteVerifyAPI = "https://hcaptcha.com/siteverify"
						cfg.ClientAPI.RecaptchaApiJsUrl = "https://js.hcaptcha.com/1/api.js"
						cfg.ClientAPI.RecaptchaFormField = "h-captcha-response"
						cfg.ClientAPI.RecaptchaSitekeyClass = "h-captcha"
					}
					cfgErrs := &config.ConfigErrors{}
					cfg.ClientAPI.Verify(cfgErrs)
					if len(*cfgErrs) > 0 {
						t.Fatalf("(hCaptcha=%v) unexpected config errors: %s", useHCaptcha, cfgErrs.Error())
					}

					req := httptest.NewRequest(http.MethodGet, "/?session=1337", nil)
					rec := httptest.NewRecorder()

					AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
					if !recaptchaEnabled {
						if rec.Code != http.StatusBadRequest {
							t.Fatalf("unexpected response code: %d, want %d", rec.Code, http.StatusBadRequest)
						}
						if rec.Body.String() != "Recaptcha login is disabled on this Homeserver" {
							t.Fatalf("unexpected response body: %s", rec.Body.String())
						}
					} else {
						if !strings.Contains(rec.Body.String(), cfg.ClientAPI.RecaptchaSitekeyClass) {
							t.Fatalf("body does not contain %s: %s", cfg.ClientAPI.RecaptchaSitekeyClass, rec.Body.String())
						}
					}

					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if wantErr {
							_, _ = w.Write([]byte(`{"success":false}`))
							return
						}
						_, _ = w.Write([]byte(`{"success":true}`))
					}))
					defer srv.Close() // nolint: errcheck

					cfg.ClientAPI.RecaptchaSiteVerifyAPI = srv.URL

					// check the result after sending the captcha
					req = httptest.NewRequest(http.MethodPost, "/?session=1337", nil)
					req.Form = url.Values{}
					req.Form.Add(cfg.ClientAPI.RecaptchaFormField, "someRandomValue")
					rec = httptest.NewRecorder()
					AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
					if recaptchaEnabled {
						if !wantErr {
							if rec.Code != http.StatusOK {
								t.Fatalf("unexpected response code: %d, want %d", rec.Code, http.StatusOK)
							}
							if rec.Body.String() != successTemplate {
								t.Fatalf("unexpected response: %s, want %s", rec.Body.String(), successTemplate)
							}
						} else {
							if rec.Code != http.StatusUnauthorized {
								t.Fatalf("unexpected response code: %d, want %d", rec.Code, http.StatusUnauthorized)
							}
							wantString := "Authentication"
							if !strings.Contains(rec.Body.String(), wantString) {
								t.Fatalf("expected response to contain '%s', but didn't: %s", wantString, rec.Body.String())
							}
						}
					} else {
						if rec.Code != http.StatusBadRequest {
							t.Fatalf("unexpected response code: %d, want %d", rec.Code, http.StatusBadRequest)
						}
						if rec.Body.String() != "Recaptcha login is disabled on this Homeserver" {
							t.Fatalf("unexpected response: %s, want %s", rec.Body.String(), "successTemplate")
						}
					}
				})
			}
		}
	}

	t.Run("unknown fallbacks are handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/?session=1337", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, "DoesNotExist", &cfg.ClientAPI)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusNotImplemented)
		}
	})

	t.Run("unknown methods are handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/?session=1337", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("missing session parameter is handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("missing session parameter is handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("missing 'response' is handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/?session=1337", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &cfg.ClientAPI)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}
