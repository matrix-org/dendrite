package routing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// The data used to populate the /consent request
type constentTemplateData struct {
	User          string
	Version       string
	UserHMAC      string
	HasConsented  bool
	PublicVersion bool
}

func consent(userAPI userapi.UserInternalAPI, cfg *config.ClientAPI) http.HandlerFunc {
	consentCfg := cfg.Matrix.UserConsentOptions
	return func(writer http.ResponseWriter, req *http.Request) {
		if !consentCfg.Enabled() {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte("consent tracking is disabled"))
			return
		}
		// The data used to populate the /consent request
		data := constentTemplateData{
			User:     req.FormValue("u"),
			Version:  req.FormValue("v"),
			UserHMAC: req.FormValue("h"),
		}
		switch req.Method {
		case http.MethodGet:
			// display the privacy policy without a form
			data.PublicVersion = data.User == "" || data.UserHMAC == "" || data.Version == ""

			// let's see if the user already consented to the current version
			if !data.PublicVersion {
				res := &userapi.QueryPolicyVersionResponse{}
				localPart, _, err := gomatrixserverlib.SplitID('@', data.User)
				if err != nil {
					logrus.WithError(err).Error("unable to print consent template")
					return
				}
				if err = userAPI.QueryPolicyVersion(req.Context(), &userapi.QueryPolicyVersionRequest{
					LocalPart: localPart,
				}, res); err != nil {
					logrus.WithError(err).Error("unable to print consent template")
					return
				}
				data.HasConsented = res.PolicyVersion == consentCfg.Version
			}

			err := consentCfg.Templates.ExecuteTemplate(writer, consentCfg.Version+".gohtml", data)
			if err != nil {
				logrus.WithError(err).Error("unable to print consent template")
				return
			}
		case http.MethodPost:
			localPart, _, err := gomatrixserverlib.SplitID('@', data.User)
			if err != nil {
				logrus.WithError(err).Error("unable to split username")
				return
			}

			ok, err := validHMAC(data.User, data.UserHMAC, consentCfg.FormSecret)
			if err != nil || !ok {
				writer.WriteHeader(http.StatusBadRequest)
				_, err = writer.Write([]byte("invalid HMAC provided"))
				if err != nil {
					return
				}
				return
			}
			if err := userAPI.PerformUpdatePolicyVersion(
				req.Context(),
				&userapi.UpdatePolicyVersionRequest{
					PolicyVersion: data.Version,
					LocalPart:     localPart,
				},
				&userapi.UpdatePolicyVersionResponse{},
			); err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				_, err = writer.Write([]byte("unable to update database"))
				if err != nil {
					logrus.WithError(err).Error("unable to write to database")
				}
				return
			}
			// display the privacy policy without a form
			data.PublicVersion = false
			data.HasConsented = true

			err = consentCfg.Templates.ExecuteTemplate(writer, consentCfg.Version+".gohtml", data)
			if err != nil {
				logrus.WithError(err).Error("unable to print consent template")
				return
			}
		}
	}
}

func validHMAC(username, userHMAC, secret string) (bool, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(username))
	if err != nil {
		return false, err
	}
	expectedMAC := mac.Sum(nil)
	decoded, err := hex.DecodeString(userHMAC)
	if err != nil {
		return false, err
	}
	return hmac.Equal(decoded, expectedMAC), nil
}
