package authorization

import (
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/setup/config"
	_ "github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/zion"
	log "github.com/sirupsen/logrus"
)

func NewRoomserverAuthorization(cfg *config.ClientAPI, rsAPI zion.RoomserverStoreAPI) authorization.Authorization {
	// Load authorization manager for Zion
	if cfg.PublicKeyAuthentication.Ethereum.GetEnableAuthZ() {
		auth, err := zion.NewZionAuthorization(cfg, rsAPI)

		if err != nil {
			log.Errorln("Failed to initialise Zion authorization manager. Using default.", err)
		} else {
			return auth
		}
	}

	return &authorization.DefaultAuthorization{}
}
