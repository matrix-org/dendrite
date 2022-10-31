package zion

import (
	"encoding/json"
)

type SpaceManagerContractAddresses struct {
	Spacemanager string `json:"spaceManager"`
	Usergranted  string `json:"usergranted"`
	Tokengranted string `json:"tokengranted"`
}

func loadSpaceManagerAddresses(byteValue []byte) (*SpaceManagerContractAddresses, error) {
	var addresses SpaceManagerContractAddresses

	err := json.Unmarshal(byteValue, &addresses)
	if err != nil {
		return nil, err
	}

	return &addresses, nil
}
