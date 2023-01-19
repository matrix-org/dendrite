package authorization

import (
	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/zion"
	log "github.com/sirupsen/logrus"
)

func NewRoomserverAuthorization(cfg *config.ClientAPI, roomQueryAPI roomserver.QueryEventsAPI) authorization.Authorization {
	// Load authorization manager for Zion
	if cfg.PublicKeyAuthentication.Ethereum.GetEnableAuthZ() {
		auth, err := zion.NewZionAuthorization(cfg, roomQueryAPI)

		if err != nil {
			log.Errorf("failed to initialise Zion authorization manager, using default. Error: %v", err)
		} else {
			return auth
		}
	}

	return &authorization.DefaultAuthorization{}
}
