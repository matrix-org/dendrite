package zion

import (
	"encoding/json"
)

type SpaceManagerContractAddresses struct {
	Spacemanager string `json:"spaceManager"`
	Usergranted  string `json:"usergranted"`
	Tokengranted string `json:"tokengranted"`
}

type SpaceFactoryContractAddress struct {
	SpaceFactory string `json:"spaceFactory"`
}

func loadSpaceFactoryAddress(byteValue []byte) (*SpaceFactoryContractAddress, error) {
	var address SpaceFactoryContractAddress
	err := json.Unmarshal(byteValue, &address)
	if err != nil {
		return nil, err
	}
	return &address, nil
}
