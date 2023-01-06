package zion

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/zion/contracts/localhost_space"
	"github.com/matrix-org/dendrite/zion/contracts/localhost_space_factory"

	//goerli_space_factory "github.com/matrix-org/dendrite/zion/contracts/goerli_space_factory" // v2 smart contract
	//goerli_space "github.com/matrix-org/dendrite/zion/contracts/goerli_space" // v2 smart contract

	log "github.com/sirupsen/logrus"
)

// v2 smart contract addresses
//
//go:embed contracts/localhost_space_factory/space-factory.json
var localhostSpaceFactoryJson []byte

// var goerliSpaceFactoryJson []byte

// v2 smart contracts
type ZionAuthorizationV2 struct {
	chainId               int
	ethClient             *ethclient.Client
	store                 Store
	localhostSpaceFactory *localhost_space_factory.LocalhostSpaceFactory
	localhostSpaces       map[string]*localhost_space.LocalhostSpace
	//goerliSpaceFactory    *goerli_space_factory.LocalhostSpaceFactory
	//goerliSpaces          map[string]*goerli_space.LocalhostSpace
}

func NewZionAuthorizationV2(chainId int, ethClient *ethclient.Client, store Store) (authorization.Authorization, error) {
	za := &ZionAuthorizationV2{
		chainId:   chainId,
		ethClient: ethClient,
		store:     store,
	}
	switch za.chainId {
	case 1337, 31337:
		localhost, err := newSpaceFactoryLocalhost(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
			return nil, err
		}
		za.localhostSpaceFactory = localhost
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		log.Error(errMsg)
		return nil, errors.New(errMsg)
	}
	// no errors.
	return za, nil
}

func newSpaceFactoryLocalhost(ethClient *ethclient.Client) (*localhost_space_factory.LocalhostSpaceFactory, error) {
	jsonAddress, err := loadSpaceFactoryAddress(localhostSpaceFactoryJson)
	if err != nil {
		return nil, err
	}
	address := common.HexToAddress(jsonAddress.SpaceFactory)
	spaceFactory, err := localhost_space_factory.NewLocalhostSpaceFactory(address, ethClient)
	if err != nil {
		return nil, err
	}
	return spaceFactory, nil
}

func (za *ZionAuthorizationV2) IsAllowed(args authorization.AuthorizationArgs) (bool, error) {
	userIdentifier := CreateUserIdentifier(args.UserId)

	// Find out if roomId is a space or a channel.
	roomInfo := za.store.GetRoomInfo(args.RoomId, userIdentifier)

	// Owner of the space / channel is always allowed to proceed.
	if roomInfo.IsOwner {
		return true, nil
	}

	switch za.chainId {
	case 1337, 31337:
		isAllowed, err := za.isAllowedLocalhost(roomInfo, userIdentifier.AccountAddress, args.Permission)
		return isAllowed, err
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		return false, errors.New(errMsg)
	}
}

func (za *ZionAuthorizationV2) getSpaceLocalhost(networkId string) (*localhost_space.LocalhostSpace, error) {
	space := za.localhostSpaces[networkId]
	if space == nil {
		// convert the networkId to keccak256 spaceIdHash
		spaceIdHash := NetworkIdToHash(networkId)
		// then use the spaceFactory to fetch the space address
		spaceAddress, err := za.localhostSpaceFactory.SpaceByHash(nil, spaceIdHash)
		if err != nil {
			return nil, err
		}
		// cache the space for future use
		space, err = localhost_space.NewLocalhostSpace(spaceAddress, za.ethClient)
		if err != nil {
			return nil, err
		}
		za.localhostSpaces[networkId] = space
	}
	return space, nil
}

func (za *ZionAuthorizationV2) isSpaceChannelDisabledLocalhost(roomInfo RoomInfo) (bool, error) {
	if za.localhostSpaceFactory == nil {
		return false, errors.New("error fetching localhost space factory contract")
	}

	space, err := za.getSpaceLocalhost(roomInfo.SpaceNetworkId)
	if err != nil {
		return false, err
	}

	switch roomInfo.ChannelNetworkId {
	case "":
		isDisabled, err := space.Disabled(nil)
		return isDisabled, err
	default:
		channelIdHash := NetworkIdToHash(roomInfo.ChannelNetworkId)
		channel, err := space.GetChannelByHash(nil, channelIdHash)
		return channel.Disabled, err
	}
}

func (za *ZionAuthorizationV2) isAllowedLocalhost(
	roomInfo RoomInfo,
	user common.Address,
	permission authorization.Permission,
) (bool, error) {
	if za.localhostSpaceFactory != nil {
		return false, errors.New("localhost SpaceFactory is not initialised")
	}

	// check if space / channel is disabled.
	disabled, err := za.isSpaceChannelDisabledLocalhost(roomInfo)
	if disabled {
		return false, ErrSpaceDisabled
	} else if err != nil {
		return false, err
	}

	// get the space and check if user is entitled.
	space, err := za.getSpaceLocalhost(roomInfo.SpaceNetworkId)
	if err != nil {
		return false, err
	}

	isEntitled := false // default to false always.
	switch roomInfo.ChannelNetworkId {
	case "":
		// ChannelNetworkId is empty. Space entitlement check.
		isEntitled, err = space.IsEntitledToSpace(
			nil,
			user,
			permission.String(),
		)
		if err != nil {
			return false, err
		}
	default:
		// channel entitlement check
		isEntitled, err = space.IsEntitledToChannel(
			nil,
			roomInfo.ChannelNetworkId,
			user,
			permission.String(),
		)
		if err != nil {
			return false, err
		}
	}

	return isEntitled, nil
}
