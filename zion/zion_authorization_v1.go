package zion

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/zion/contracts/zion_goerli"    // V1 smart contract
	"github.com/matrix-org/dendrite/zion/contracts/zion_localhost" // v1 smart contract
	log "github.com/sirupsen/logrus"
)

// v1 smart contract addresses
//
//go:embed contracts/zion_localhost/space-manager.json
var localhostSpaceManagerJson []byte

//go:embed contracts/zion_goerli/space-manager.json
var goerliSpaceManagerJson []byte

// v1 smart contracts. Deprecating.
type ZionAuthorizationV1 struct {
	chainId               int
	ethClient             *ethclient.Client
	spaceManagerLocalhost *zion_localhost.ZionSpaceManagerLocalhost
	spaceManagerGoerli    *zion_goerli.ZionSpaceManagerGoerli
	store                 Store
}

func NewZionAuthorizationV1(chainId int, ethClient *ethclient.Client, store Store) (authorization.Authorization, error) {
	za := &ZionAuthorizationV1{
		chainId:   chainId,
		ethClient: ethClient,
		store:     store,
	}
	switch za.chainId {
	case 1337, 31337:
		localhost, err := newZionSpaceManagerLocalhost(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
			return nil, err
		}
		za.spaceManagerLocalhost = localhost
	case 5:
		goerli, err := newZionSpaceManagerGoerli(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerGoerli", err)
			return nil, err
		}
		za.spaceManagerGoerli = goerli
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		log.Error(errMsg)
		return nil, errors.New(errMsg)
	}
	// no errors.
	return za, nil
}

func newZionSpaceManagerLocalhost(ethClient *ethclient.Client) (*zion_localhost.ZionSpaceManagerLocalhost, error) {
	jsonAddresses, err := loadSpaceManagerAddresses(localhostSpaceManagerJson)
	if err != nil {
		return nil, err
	}
	address := common.HexToAddress(jsonAddresses.Spacemanager)
	spaceManager, err := zion_localhost.NewZionSpaceManagerLocalhost(address, ethClient)
	if err != nil {
		return nil, err
	}
	return spaceManager, nil
}

func newZionSpaceManagerGoerli(ethClient *ethclient.Client) (*zion_goerli.ZionSpaceManagerGoerli, error) {
	addresses, err := loadSpaceManagerAddresses(goerliSpaceManagerJson)
	if err != nil {
		return nil, err
	}
	address := common.HexToAddress(addresses.Spacemanager)
	spaceManager, err := zion_goerli.NewZionSpaceManagerGoerli(address, ethClient)
	if err != nil {
		return nil, err
	}
	return spaceManager, nil
}

func (za *ZionAuthorizationV1) IsAllowed(args authorization.AuthorizationArgs) (bool, error) {
	userIdentifier := CreateUserIdentifier(args.UserId)

	// Find out if roomId is a space or a channel.
	roomInfo := za.store.GetRoomInfo(args.RoomId, userIdentifier)

	// Owner of the space / channel is always allowed to proceed.
	if roomInfo.IsOwner {
		return true, nil
	}

	switch za.chainId {
	case 1337, 31337:
		// Check if space / channel is disabled.
		disabled, err := za.isSpaceChannelDisabledLocalhost(roomInfo)
		if disabled {
			return false, ErrSpaceDisabled
		} else if err != nil {
			return false, err
		}
		isAllowed, err := za.isAllowedLocalhost(roomInfo, userIdentifier.AccountAddress, args.Permission)
		return isAllowed, err
	case 5:
		// Check if space / channel is disabled.
		disabled, err := za.isSpaceChannelDisabledGoerli(roomInfo)
		if disabled {
			return false, ErrChannelDisabled
		} else if err != nil {
			return false, err
		}
		return za.isAllowedGoerli(roomInfo, userIdentifier.AccountAddress, args.Permission)
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		return false, errors.New(errMsg)
	}
}

func (za *ZionAuthorizationV1) isSpaceChannelDisabledLocalhost(roomInfo RoomInfo) (bool, error) {
	if za.spaceManagerLocalhost == nil {
		return false, errors.New("error fetching localhost space manager contract")
	}
	switch roomInfo.ChannelNetworkId {
	case "":
		spInfo, err := za.spaceManagerLocalhost.GetSpaceInfoBySpaceId(nil, roomInfo.SpaceNetworkId)
		return spInfo.Disabled, err
	default:
		chInfo, err := za.spaceManagerLocalhost.GetChannelInfoByChannelId(nil, roomInfo.SpaceNetworkId, roomInfo.ChannelNetworkId)
		return chInfo.Disabled, err
	}
}

func (za *ZionAuthorizationV1) isSpaceChannelDisabledGoerli(roomInfo RoomInfo) (bool, error) {
	if za.spaceManagerGoerli == nil {
		return false, errors.New("error fetching goerli space manager contract")
	}
	switch roomInfo.ChannelNetworkId {
	case "":
		spInfo, err := za.spaceManagerGoerli.GetSpaceInfoBySpaceId(nil, roomInfo.SpaceNetworkId)
		return spInfo.Disabled, err
	default:
		chInfo, err := za.spaceManagerGoerli.GetChannelInfoByChannelId(nil, roomInfo.SpaceNetworkId, roomInfo.ChannelNetworkId)
		return chInfo.Disabled, err
	}
}

func (za *ZionAuthorizationV1) isAllowedLocalhost(
	roomInfo RoomInfo,
	user common.Address,
	_permission authorization.Permission,
) (bool, error) {
	if za.spaceManagerLocalhost == nil {
		return false, errors.New("error fetching localhost space manager contract")
	}

	permission := zion_localhost.DataTypesPermission{
		Name: _permission.String(),
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

func (za *ZionAuthorizationV1) isAllowedGoerli(
	roomInfo RoomInfo,
	user common.Address,
	_permission authorization.Permission,
) (bool, error) {
	if za.spaceManagerGoerli == nil {
		return false, errors.New("error fetching goerli space manager contract")
	}

	permission := zion_goerli.DataTypesPermission{
		Name: _permission.String(),
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
