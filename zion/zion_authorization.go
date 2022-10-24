package zion

import (
	_ "embed"

	"github.com/ethereum/go-ethereum/common"
	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	zion_goerli "github.com/matrix-org/dendrite/zion/contracts/goerli/zion_goerli"
	zion_localhost "github.com/matrix-org/dendrite/zion/contracts/localhost/zion_localhost"
	log "github.com/sirupsen/logrus"
)

//go:embed contracts/localhost/addresses/space-manager.json
var localhostJson []byte

//go:embed contracts/goerli/addresses/space-manager.json
var goerliJson []byte

type ZionAuthorization struct {
	store                 Store
	spaceManagerLocalhost *zion_localhost.ZionSpaceManagerLocalhost
	spaceManagerGoerli    *zion_goerli.ZionSpaceManagerGoerli
	chainId               int
}

func NewZionAuthorization(
	cfg *config.ClientAPI,
	rsAPI roomserver.ClientRoomserverAPI,
) (authorization.Authorization, error) {
	if cfg.PublicKeyAuthentication.Ethereum.NetworkUrl == "" {
		log.Errorf("No blockchain network url specified in config\n")
		return nil, nil
	}

	var auth ZionAuthorization

	auth.chainId = cfg.PublicKeyAuthentication.Ethereum.ChainID
	auth.store = NewStore(rsAPI)

	switch auth.chainId {
	case 1337, 31337:
		localhost, err := newZionSpaceManagerLocalhost(cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
		}
		auth.spaceManagerLocalhost = localhost

	case 5:
		goerli, err := newZionSpaceManagerGoerli(cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerGoerli", err)
		}
		auth.spaceManagerGoerli = goerli

	default:
		log.Errorf("Unsupported chain id: %d\n", auth.chainId)
	}

	return &auth, nil
}

func (za *ZionAuthorization) IsAllowed(args authorization.AuthorizationArgs) (bool, error) {
	userIdentifier := CreateUserIdentifier(args.UserId)

	// Find out if roomId is a space or a channel.
	roomInfo := za.store.GetRoomInfo(args.RoomId, userIdentifier)

	// Owner of the space / channel is always allowed to proceed.
	if roomInfo.IsOwner {
		return true, nil
	}

	switch za.chainId {
	case 1337, 31337:
		return za.isAllowedLocalhost(roomInfo, userIdentifier.AccountAddress, args.Permission)
	case 5:
		return za.isAllowedGoerli(roomInfo, userIdentifier.AccountAddress, args.Permission)
	default:
		log.Errorf("Unsupported chain id: %d", userIdentifier.ChainId)
	}

	return false, nil
}

func (za *ZionAuthorization) isAllowedLocalhost(
	roomInfo RoomInfo,
	user common.Address,
	permission authorization.Permission,
) (bool, error) {
	if za.spaceManagerLocalhost != nil {
		permission := zion_localhost.DataTypesPermission{
			Name: permission.String(),
		}

		isEntitled, err := za.spaceManagerLocalhost.IsEntitled(
			nil,
			roomInfo.SpaceNetworkId,
			roomInfo.ChannelNetworkId,
			user,
			permission,
		)

		if err != nil {
			return false, err
		}

		return isEntitled, nil
	}

	return false, nil
}

func (za *ZionAuthorization) isAllowedGoerli(
	roomInfo RoomInfo,
	user common.Address,
	permission authorization.Permission,
) (bool, error) {
	if za.spaceManagerGoerli != nil {
		permission := zion_goerli.DataTypesPermission{
			Name: permission.String(),
		}

		isEntitled, err := za.spaceManagerGoerli.IsEntitled(
			nil,
			roomInfo.SpaceNetworkId,
			roomInfo.ChannelNetworkId,
			user,
			permission,
		)

		if err != nil {
			return false, err
		}

		return isEntitled, nil
	}

	return false, nil
}

func newZionSpaceManagerLocalhost(endpointUrl string) (*zion_localhost.ZionSpaceManagerLocalhost, error) {
	addresses, err := loadSpaceManagerAddresses(localhostJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress(addresses.Spacemanager)

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := zion_localhost.NewZionSpaceManagerLocalhost(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}

func newZionSpaceManagerGoerli(endpointUrl string) (*zion_goerli.ZionSpaceManagerGoerli, error) {
	addresses, err := loadSpaceManagerAddresses(goerliJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress((addresses.Spacemanager))

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := zion_goerli.NewZionSpaceManagerGoerli(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}
