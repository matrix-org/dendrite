package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
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
		switch req.Method {
		case http.MethodGet:
			// The data used to populate the /consent request
			data := constentTemplateData{
				User:     req.FormValue("u"),
				Version:  req.FormValue("v"),
				UserHMAC: req.FormValue("h"),
			}
			// display the privacy policy without a form
			data.PublicVersion = data.User == "" || data.UserHMAC == "" || data.Version == ""

			err := consentCfg.Templates.ExecuteTemplate(writer, consentCfg.Version+".gohtml", data)
			if err != nil {
				logrus.WithError(err).Error("unable to print consent template")
			}
		case http.MethodPost:

		}
	}
}
