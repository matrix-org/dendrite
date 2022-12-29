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
	zion_goerli "github.com/matrix-org/dendrite/zion/contracts/zion_goerli"       // V1 smart contract
	zion_localhost "github.com/matrix-org/dendrite/zion/contracts/zion_localhost" // v1 smart contract

	//goerli_space_factory "github.com/matrix-org/dendrite/zion/contracts/goerli_space_factory" // v2 smart contract
	//goerli_space "github.com/matrix-org/dendrite/zion/contracts/goerli_space" // v2 smart contract
	// v2 smart contract
	localhost_space_factory "github.com/matrix-org/dendrite/zion/contracts/localhost_space_factory" // v2 smart contract
	log "github.com/sirupsen/logrus"
)

// V1 smart contract addresses
//
//go:embed contracts/zion_localhost/space-manager.json
var localhostSpaceManagerJson []byte

//go:embed contracts/zion_goerli/space-manager.json
var goerliSpaceManagerJson []byte

// V2 smart contract addresses
//
//go:embed contracts/localhost_space_factory/space-factory.json
var localhostSpaceFactoryJson []byte

// var goerliSpaceFactoryJson []byte

var ErrSpaceDisabled = errors.New("space disabled")
var ErrChannelDisabled = errors.New("channel disabled")

type ZionAuthorization struct {
	chainId               int
	ethClient             *ethclient.Client
	spaceManagerLocalhost *zion_localhost.ZionSpaceManagerLocalhost      // v1 smart contract. Deprecating.
	spaceManagerGoerli    *zion_goerli.ZionSpaceManagerGoerli            // v1 smart contract. Deprecating.
	localhostSpaceFactory *localhost_space_factory.LocalhostSpaceFactory // v2 localhost SpaceFactory smart contract
	//localhostSpaces       map[string]localhost_space.LocalhostSpace      // v2 localhost map from networkId to a space contract
	//goerliSpaceFactory    *goerli_space_factory.LocalhostSpaceFactory    // v2 goerli SpaceFactory smart contract
	//goerliSpaces          map[string]goerli_space.LocalhostSpace         // v2 goerli map from networkId to a space contract
	store Store
}

func NewZionAuthorization(cfg *config.ClientAPI, roomQueryAPI roomserver.QueryEventsAPI) (authorization.Authorization, error) {
	if cfg.PublicKeyAuthentication.Ethereum.NetworkUrl == "" {
		log.Errorf("No blockchain network url specified in config\n")
		return nil, nil
	}

	ethClient, err := GetEthClient(cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
	if err != nil {
		log.Errorf("Cannot connect to eth client %v\n", cfg.PublicKeyAuthentication.Ethereum.NetworkUrl)
		return nil, err
	}

	auth := ZionAuthorization{
		store:     NewStore(roomQueryAPI),
		chainId:   cfg.PublicKeyAuthentication.Ethereum.GetChainID(),
		ethClient: ethClient,
	}

	if cfg.PublicKeyAuthentication.Ethereum.ContractVersion == "v2" {
		err := auth.initializeContractsV2()
		if err != nil {
			return nil, err
		}
	} else {
		err := auth.initializeContractsV1()
		if err != nil {
			return nil, err
		}
	}

	// no errors. return the auth object.
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
		log.Errorf("Unsupported chain id: %d", userIdentifier.ChainId)
	}

	return false, nil
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

func (za *ZionAuthorization) initializeContractsV1() error {
	switch za.chainId {
	case 1337, 31337:
		localhost, err := newZionSpaceManagerLocalhost(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
			return err
		}
		za.spaceManagerLocalhost = localhost

	case 5:
		goerli, err := newZionSpaceManagerGoerli(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerGoerli", err)
			return err
		}
		za.spaceManagerGoerli = goerli
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		log.Error(errMsg)
		return errors.New(errMsg)
	}
	// no errors.
	return nil
}

func (za *ZionAuthorization) initializeContractsV2() error {
	switch za.chainId {
	case 1337, 31337:
		localhost, err := newSpaceFactoryLocalhost(za.ethClient)
		if err != nil {
			log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
			return err
		}
		za.localhostSpaceFactory = localhost
	default:
		errMsg := fmt.Sprintf("Unsupported chain id: %d", za.chainId)
		log.Error(errMsg)
		return errors.New(errMsg)
	}
	// no errors.
	return nil
}

func (za *ZionAuthorization) isSpaceChannelDisabledLocalhost(roomInfo RoomInfo) (bool, error) {
	if za.spaceManagerLocalhost == nil {
		return false, errors.New("error fetching space manager contract")
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

func (za *ZionAuthorization) isSpaceChannelDisabledGoerli(roomInfo RoomInfo) (bool, error) {
	if za.spaceManagerGoerli == nil {
		return false, errors.New("error fetching space manager contract")
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
