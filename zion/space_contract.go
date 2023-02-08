package zion

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/matrix-org/dendrite/authorization"
)

type SpaceContract interface {
	IsSpaceDisabled(spaceNetworkId string) (bool, error)
	IsChannelDisabled(
		spaceNetworkId string,
		channelNetworkId string,
	) (bool, error)
	IsEntitledToSpace(
		spaceNetworkId string,
		user common.Address,
		permission authorization.Permission,
	) (bool, error)
	IsEntitledToChannel(
		spaceNetworkId string,
		channelNetworkId string,
		user common.Address,
		permission authorization.Permission,
	) (bool, error)
}
