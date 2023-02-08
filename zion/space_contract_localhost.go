package zion

import (
	_ "embed"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/zion/contracts/localhost_space"
	"github.com/matrix-org/dendrite/zion/contracts/localhost_space_factory"
	log "github.com/sirupsen/logrus"
)

//go:embed contracts/localhost_space_factory/space-factory.json
var localhostSpaceFactoryJson []byte

type SpaceContractLocalhost struct {
	ethClient    *ethclient.Client
	spaceFactory *localhost_space_factory.LocalhostSpaceFactory
	spaces       map[string]*localhost_space.LocalhostSpace
}

func NewSpaceContractLocalhost(ethClient *ethclient.Client) (*SpaceContractLocalhost, error) {
	jsonAddress, err := loadSpaceFactoryAddress(localhostSpaceFactoryJson)
	if err != nil {
		log.Errorf("error parsing localhost space factory contract address %v. Error: %v", jsonAddress, err)
		return nil, err
	}
	address := common.HexToAddress(jsonAddress.SpaceFactory)
	spaceFactory, err := localhost_space_factory.NewLocalhostSpaceFactory(address, ethClient)
	if err != nil {
		log.Errorf("error fetching localhost space factory contract with address %v. Error: %v", jsonAddress, err)
		return nil, err
	}
	// no errors.
	var spaceContract = &SpaceContractLocalhost{
		ethClient:    ethClient,
		spaceFactory: spaceFactory,
		spaces:       make(map[string]*localhost_space.LocalhostSpace),
	}
	return spaceContract, nil
}

func (za *SpaceContractLocalhost) IsEntitledToSpace(
	spaceNetworkId string,
	user common.Address,
	permission authorization.Permission,
) (bool, error) {
	// get the space and check if user is entitled.
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}
	isEntitled, err := space.IsEntitledToSpace(
		nil,
		user,
		permission.String(),
	)
	return isEntitled, err
}

func (za *SpaceContractLocalhost) IsEntitledToChannel(
	spaceNetworkId string,
	channelNetworkId string,
	user common.Address,
	permission authorization.Permission,
) (bool, error) {
	// get the space and check if user is entitled.
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}
	// channel entitlement check
	isEntitled, err := space.IsEntitledToChannel(
		nil,
		channelNetworkId,
		user,
		permission.String(),
	)
	return isEntitled, err
}

func (za *SpaceContractLocalhost) IsSpaceDisabled(spaceNetworkId string) (bool, error) {
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}

	isDisabled, err := space.Disabled(nil)
	return isDisabled, err
}

func (za *SpaceContractLocalhost) IsChannelDisabled(spaceNetworkId string, channelNetworkId string) (bool, error) {
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}
	channelIdHash := NetworkIdToHash(channelNetworkId)
	channel, err := space.GetChannelByHash(nil, channelIdHash)
	return channel.Disabled, err
}

func (za *SpaceContractLocalhost) getSpace(networkId string) (*localhost_space.LocalhostSpace, error) {
	if za.spaces[networkId] == nil {
		// convert the networkId to keccak256 spaceIdHash
		spaceIdHash := NetworkIdToHash(networkId)
		// then use the spaceFactory to fetch the space address
		spaceAddress, err := za.spaceFactory.SpaceByHash(nil, spaceIdHash)
		if err != nil {
			return nil, err
		}
		// cache the space for future use
		space, err := localhost_space.NewLocalhostSpace(spaceAddress, za.ethClient)
		if err != nil {
			return nil, err
		}
		za.spaces[networkId] = space
	}
	return za.spaces[networkId], nil
}
