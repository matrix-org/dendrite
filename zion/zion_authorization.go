package zion

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"

	log "github.com/sirupsen/logrus"
)

var ErrSpaceDisabled = errors.New("space disabled")
var ErrChannelDisabled = errors.New("channel disabled")

type ZionAuthorization struct {
	chainId       int
	ethClient     *ethclient.Client
	store         Store
	spaceContract SpaceContract
}

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

	za := &ZionAuthorization{
		chainId:   chainId,
		ethClient: ethClient,
		store:     store,
	}
	switch za.chainId {
	case 1337, 31337:
		localhost, err := NewSpaceContractLocalhost(za.ethClient)
		if err != nil {
			log.Errorf("error instantiating SpaceContractLocalhost. Error: %v", err)
			return nil, err
		}
		za.spaceContract = localhost
	case 5:
		goerli, err := NewSpaceContractGoerli(za.ethClient)
		if err != nil {
			log.Errorf("error instantiating SpaceContractGoerli. Error: %v", err)
			return nil, err
		}
		za.spaceContract = goerli
	default:
		errMsg := fmt.Sprintf("unsupported chain id: %d", za.chainId)
		log.Error(errMsg)
		return nil, errors.New(errMsg)
	}
	// no errors.
	return za, nil
}

func (za *ZionAuthorization) IsAllowed(args authorization.AuthorizationArgs) (bool, error) {
	userIdentifier := CreateUserIdentifier(args.UserId)

	// Find out if roomId is a space or a channel.
	roomInfo := za.store.GetRoomInfo(args.RoomId, userIdentifier)

	// Owner of the space / channel is always allowed to proceed.
	if roomInfo.IsOwner {
		return true, nil
	}

	// Check if user is entitled to space / channel.
	switch roomInfo.RoomType {
	case Space:
		isEntitled, err := za.isEntitledToSpace(roomInfo, userIdentifier.AccountAddress, args.Permission)
		return isEntitled, err
	case Channel:
		isEntitled, err := za.isEntitledToChannel(roomInfo, userIdentifier.AccountAddress, args.Permission)
		return isEntitled, err
	default:
		errMsg := fmt.Sprintf("unhandled room type: %s", roomInfo.RoomType)
		log.Error("IsAllowed", errMsg)
		return false, errors.New(errMsg)
	}
}

func (za *ZionAuthorization) isEntitledToSpace(roomInfo RoomInfo, user common.Address, permission authorization.Permission) (bool, error) {
	// space disabled check.
	isDisabled, err := za.spaceContract.IsSpaceDisabled(roomInfo.SpaceNetworkId)
	if err != nil {
		return false, err
	} else if isDisabled {
		return false, ErrSpaceDisabled
	}

	// space entitlement check.
	isEntitled, err := za.spaceContract.IsEntitledToSpace(
		roomInfo.SpaceNetworkId,
		user,
		permission,
	)
	return isEntitled, err
}

func (za *ZionAuthorization) isEntitledToChannel(roomInfo RoomInfo, user common.Address, permission authorization.Permission) (bool, error) {
	// channel disabled check.
	isDisabled, err := za.spaceContract.IsChannelDisabled(roomInfo.SpaceNetworkId, roomInfo.ChannelNetworkId)
	if err != nil {
		return false, err
	} else if isDisabled {
		return false, ErrSpaceDisabled
	}

	// channel entitlement check.
	isEntitled, err := za.spaceContract.IsEntitledToChannel(
		roomInfo.SpaceNetworkId,
		roomInfo.ChannelNetworkId,
		user,
		permission,
	)
	return isEntitled, err
}
