package zion

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// DataTypesCreateSpaceData is an auto generated low-level Go binding around an user-defined struct.
type DataTypesCreateSpaceData struct {
	SpaceName string
	NetworkId string
}

// DataTypesCreateSpaceTokenEntitlementData is an auto generated low-level Go binding around an user-defined struct.
type DataTypesCreateSpaceTokenEntitlementData struct {
	EntitlementModuleAddress common.Address
	TokenAddress             common.Address
	Quantity                 *big.Int
	Description              string
	EntitlementTypes         []uint8
}

// DataTypesEntitlementModuleInfo is an auto generated low-level Go binding around an user-defined struct.
type DataTypesEntitlementModuleInfo struct {
	EntitlementAddress     common.Address
	EntitlementName        string
	EntitlementDescription string
}

// DataTypesSpaceInfo is an auto generated low-level Go binding around an user-defined struct.
type DataTypesSpaceInfo struct {
	SpaceId   *big.Int
	CreatedAt *big.Int
	Name      string
	Creator   common.Address
	Owner     common.Address
}
