package routing

import (
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

func Test_validHMAC(t *testing.T) {
	type args struct {
		username string
		userHMAC string
		secret   string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "invalid hmac",
			args:    args{},
			wantErr: false,
			want:    false,
		},
		// $ echo -n '@alice:localhost' | openssl sha256 -hmac 'helloWorld'
		//(stdin)= 121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e
		//
		{
			name: "valid hmac",
			args: args{
				username: "@alice:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				secret:   "helloWorld",
			},
			want: true,
		},
		{
			name: "invalid hmac",
			args: args{
				username: "@bob:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				secret:   "helloWorld",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validHMAC(tt.args.username, tt.args.userHMAC, tt.args.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("validHMAC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validHMAC() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type dummyAPI struct {
	usersConsent map[string]string
}

func (d dummyAPI) QueryOutdatedPolicy(ctx context.Context, req *userapi.QueryOutdatedPolicyRequest, res *userapi.QueryOutdatedPolicyResponse) error {
	return nil
}

func (d dummyAPI) PerformUpdatePolicyVersion(ctx context.Context, req *userapi.UpdatePolicyVersionRequest, res *userapi.UpdatePolicyVersionResponse) error {
	d.usersConsent[req.Localpart] = req.PolicyVersion
	return nil
}

func (d dummyAPI) QueryPolicyVersion(ctx context.Context, req *userapi.QueryPolicyVersionRequest, res *userapi.QueryPolicyVersionResponse) error {
	res.PolicyVersion = "v2.0"
	return nil
}

const dummyTemplate = `
{{ if .HasConsented }}
Consent given.
{{ else }}
WithoutForm
	{{ if not .ReadOnly }}
    With Form.
    {{ end }}
{{ end }}`

func Test_consent(t *testing.T) {
	type args struct {
		username string
		userHMAC string
		version  string
		method   string
	}
	tests := []struct {
		name             string
		args             args
		wantRespCode     int
		wantBodyContains string
	}{
		{
			name: "not a userID, valid hmac",
			args: args{
				username: "notAuserID",
				userHMAC: "7578bbface5ebb250a63935cebc05ca12060f58ebdbd271ecbc25e25a3da154d",
				version:  "v1.0",
				method:   http.MethodGet,
			},
			wantRespCode: http.StatusInternalServerError,
		},

		// $ echo -n '@alice:localhost' | openssl sha256 -hmac 'helloWorld'
		//(stdin)= 121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e
		//
		{
			name: "valid hmac for alice GET, not consented",
			args: args{
				username: "@alice:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				version:  "v1.0",
				method:   http.MethodGet,
			},
			wantRespCode:     http.StatusOK,
			wantBodyContains: "With form",
		},
		{
			name: "alice consents successfully",
			args: args{
				username: "@alice:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				version:  "v1.0",
				method:   http.MethodPost,
			},
			wantRespCode:     http.StatusOK,
			wantBodyContains: "Consent given",
		},
		{
			name: "valid hmac for alice GET, new version",
			args: args{
				username: "@alice:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				version:  "v2.0",
				method:   http.MethodGet,
			},
			wantRespCode:     http.StatusOK,
			wantBodyContains: "With form",
		},
		{
			name: "no hmac provided for alice, read only should be displayed",
			args: args{
				username: "@alice:localhost",
				userHMAC: "",
				version:  "v1.0",
				method:   http.MethodGet,
			},
			wantRespCode:     http.StatusOK,
			wantBodyContains: "WithoutForm",
		},
		{
			name: "alice trying to get bobs status is forbidden",
			args: args{
				username: "@bob:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				version:  "v1.0",
				method:   http.MethodGet,
			},
			wantRespCode:     http.StatusForbidden,
			wantBodyContains: "forbidden",
		},
		{
			name: "alice trying to consent for bob is forbidden",
			args: args{
				username: "@bob:localhost",
				userHMAC: "121c9bab767ed87a3136db0c3002144dfe414720aa328d235199082e4757541e",
				version:  "v1.0",
				method:   http.MethodPost,
			},
			wantRespCode:     http.StatusForbidden,
			wantBodyContains: "forbidden",
		},
	}

	userAPI := dummyAPI{
		usersConsent: map[string]string{},
	}
	consentTemplates := template.Must(template.New("v1.0.gohtml").Parse(dummyTemplate))
	consentTemplates = template.Must(consentTemplates.New("v2.0.gohtml").Parse(dummyTemplate))
	userconsentOpts := config.UserConsentOptions{
		FormSecret: "helloWorld",
		Version:    "v1.0",
		Templates:  consentTemplates,
		BaseURL:    "http://localhost",
	}
	cfg := &config.ClientAPI{
		Matrix: &config.Global{
			UserConsentOptions: userconsentOpts,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := fmt.Sprintf("%s/consent?u=%s&v=%s&h=%s",
				userconsentOpts.BaseURL, tt.args.username, tt.args.version, tt.args.userHMAC,
			)

			req := httptest.NewRequest(tt.args.method, url, nil)
			w := httptest.NewRecorder()

			consent(w, req, userAPI, cfg)

			resp := w.Result()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unable to read response body: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantRespCode {
				t.Fatalf("expected http %d, got %d", tt.wantRespCode, resp.StatusCode)
			}

			if !strings.Contains(strings.ToLower(string(body)), strings.ToLower(tt.wantBodyContains)) {
				t.Fatalf("expected body to contain %s, but got %s", tt.wantBodyContains, string(body))
			}
		})
	}
}
