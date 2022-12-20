package routing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test/testrig"
)

func Test_AuthFallback(t *testing.T) {
	base, _, _ := testrig.Base(nil)
	defer base.Close()

	for _, useHCaptcha := range []bool{false, true} {
		for _, recaptchaEnabled := range []bool{false, true} {
			for _, wantErr := range []bool{false, true} {
				t.Run(fmt.Sprintf("useHCaptcha(%v) - recaptchaEnabled(%v) - wantErr(%v)", useHCaptcha, recaptchaEnabled, wantErr), func(t *testing.T) {
					// Set the defaults for each test
					base.Cfg.ClientAPI.Defaults(config.DefaultOpts{Generate: true, Monolithic: true})
					base.Cfg.ClientAPI.RecaptchaEnabled = recaptchaEnabled
					base.Cfg.ClientAPI.RecaptchaPublicKey = "pub"
					base.Cfg.ClientAPI.RecaptchaPrivateKey = "priv"
					if useHCaptcha {
						base.Cfg.ClientAPI.RecaptchaSiteVerifyAPI = "https://hcaptcha.com/siteverify"
						base.Cfg.ClientAPI.RecaptchaApiJsUrl = "https://js.hcaptcha.com/1/api.js"
						base.Cfg.ClientAPI.RecaptchaFormField = "h-captcha-response"
						base.Cfg.ClientAPI.RecaptchaSitekeyClass = "h-captcha"
					}
					cfgErrs := &config.ConfigErrors{}
					base.Cfg.ClientAPI.Verify(cfgErrs, true)
					if len(*cfgErrs) > 0 {
						t.Fatalf("(hCaptcha=%v) unexpected config errors: %s", useHCaptcha, cfgErrs.Error())
					}

					req := httptest.NewRequest(http.MethodGet, "/?session=1337", nil)
					rec := httptest.NewRecorder()

					AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &base.Cfg.ClientAPI)
					if !recaptchaEnabled {
						if rec.Code != http.StatusBadRequest {
							t.Fatalf("unexpected response code: %d, want %d", rec.Code, http.StatusBadRequest)
						}
						if rec.Body.String() != "Recaptcha login is disabled on this Homeserver" {
							t.Fatalf("unexpected response body: %s", rec.Body.String())
						}
					} else {
						if !strings.Contains(rec.Body.String(), base.Cfg.ClientAPI.RecaptchaSitekeyClass) {
							t.Fatalf("body does not contain %s: %s", base.Cfg.ClientAPI.RecaptchaSitekeyClass, rec.Body.String())
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

					base.Cfg.ClientAPI.RecaptchaSiteVerifyAPI = srv.URL

					// check the result after sending the captcha
					req = httptest.NewRequest(http.MethodPost, "/?session=1337", nil)
					req.Form = url.Values{}
					req.Form.Add(base.Cfg.ClientAPI.RecaptchaFormField, "someRandomValue")
					rec = httptest.NewRecorder()
					AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &base.Cfg.ClientAPI)
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
		AuthFallback(rec, req, "DoesNotExist", &base.Cfg.ClientAPI)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusNotImplemented)
		}
	})

	t.Run("unknown methods are handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/?session=1337", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &base.Cfg.ClientAPI)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("missing session parameter is handled correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		AuthFallback(rec, req, authtypes.LoginTypeRecaptcha, &base.Cfg.ClientAPI)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected http status: %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}
