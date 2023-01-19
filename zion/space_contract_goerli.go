package zion

import (
	_ "embed"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gologme/log"
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/zion/contracts/goerli_space"
	"github.com/matrix-org/dendrite/zion/contracts/goerli_space_factory"
)

//go:embed contracts/goerli_space_factory/space-factory.json
var goerliSpaceFactoryJson []byte

type SpaceContractGoerli struct {
	ethClient    *ethclient.Client
	spaceFactory *goerli_space_factory.GoerliSpaceFactory
	spaces       map[string]*goerli_space.GoerliSpace
}

func NewSpaceContractGoerli(ethClient *ethclient.Client) (*SpaceContractGoerli, error) {
	jsonAddress, err := loadSpaceFactoryAddress(goerliSpaceFactoryJson)
	if err != nil {
		log.Errorf("error parsing goerli space factory contract address %v. Error: %v", jsonAddress, err)
		return nil, err
	}
	address := common.HexToAddress(jsonAddress.SpaceFactory)
	spaceFactory, err := goerli_space_factory.NewGoerliSpaceFactory(address, ethClient)
	if err != nil {
		log.Errorf("error fetching goerli space factory contract with address %v. Error: %v", jsonAddress, err)
		return nil, err
	}
	// no errors.
	var spaceContract = &SpaceContractGoerli{
		ethClient:    ethClient,
		spaceFactory: spaceFactory,
		spaces:       make(map[string]*goerli_space.GoerliSpace),
	}
	return spaceContract, nil
}

func (za *SpaceContractGoerli) IsEntitledToSpace(
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
	if err != nil {
		return false, err
	}
	return isEntitled, nil
}

func (za *SpaceContractGoerli) IsEntitledToChannel(
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

func (za *SpaceContractGoerli) IsSpaceDisabled(spaceNetworkId string) (bool, error) {
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}
	isDisabled, err := space.Disabled(nil)
	return isDisabled, err
}

func (za *SpaceContractGoerli) IsChannelDisabled(spaceNetworkId string, channelNetworkId string) (bool, error) {
	space, err := za.getSpace(spaceNetworkId)
	if err != nil {
		return false, err
	}
	channelIdHash := NetworkIdToHash(channelNetworkId)
	channel, err := space.GetChannelByHash(nil, channelIdHash)
	if err != nil {
		return false, err
	}
	return channel.Disabled, err
}

func (za *SpaceContractGoerli) getSpace(networkId string) (*goerli_space.GoerliSpace, error) {
	if za.spaces[networkId] == nil {
		// convert the networkId to keccak256 spaceIdHash
		spaceIdHash := NetworkIdToHash(networkId)
		// then use the spaceFactory to fetch the space address
		spaceAddress, err := za.spaceFactory.SpaceByHash(nil, spaceIdHash)
		if err != nil {
			return nil, err
		}
		// cache the space for future use
		space, err := goerli_space.NewGoerliSpace(spaceAddress, za.ethClient)
		if err != nil {
			return nil, err
		}
		za.spaces[networkId] = space
	}
	return za.spaces[networkId], nil
}
