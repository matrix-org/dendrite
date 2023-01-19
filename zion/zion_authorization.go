package zion

import (
	_ "embed"
	"errors"

	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"

	log "github.com/sirupsen/logrus"
)

var ErrSpaceDisabled = errors.New("space disabled")
var ErrChannelDisabled = errors.New("channel disabled")

func NewZionAuthorization(cfg *config.ClientAPI, roomQueryAPI roomserver.QueryEventsAPI) (authorization.Authorization, error) {
	// create the authorization states
	store := NewStore(roomQueryAPI)
	chainId := cfg.PublicKeyAuthentication.Ethereum.GetChainID()
	// initialise the eth client.
	if cfg.PublicKeyAuthentication.Ethereum.NetworkUrl == "" {
		log.Errorf("No blockchain network url specified in config\n")
		return nil, nil
	}
	ethClient, err := GetEthClient(cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
	if err != nil {
		log.Errorf("Cannot connect to eth client %v\n", cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
		return nil, err
	}
	return NewZionAuthorizationV2(
		chainId,
		ethClient,
		store,
	)
}
