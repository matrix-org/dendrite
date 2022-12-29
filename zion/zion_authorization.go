package zion

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"

	log "github.com/sirupsen/logrus"
)

type ContractVersion uint8

const (
	v1 ContractVersion = 1
	v2 ContractVersion = 2
)

var ErrSpaceDisabled = errors.New("space disabled")
var ErrChannelDisabled = errors.New("channel disabled")

func NewZionAuthorization(cfg *config.ClientAPI, roomQueryAPI roomserver.QueryEventsAPI) (authorization.Authorization, error) {
	// create the authorization states
	store := NewStore(roomQueryAPI)
	chainId := cfg.PublicKeyAuthentication.Ethereum.GetChainID()
	contractVersion := v1 // default
	if cfg.PublicKeyAuthentication.Ethereum.ContractVersion == "v2" {
		contractVersion = v2
	}

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

	switch contractVersion {
	case v1:
		return NewZionAuthorizationV1(
			chainId,
			ethClient,
			store,
		)
	case v2:
		return NewZionAuthorizationV2(
			chainId,
			ethClient,
			store,
		)
	default:
		errMsg := fmt.Sprintf("Unsupported contract version: %d", contractVersion)
		return nil, errors.New(errMsg)
	}
}
