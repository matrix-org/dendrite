package config

import (
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

type AuthParams interface {
	GetParams() interface{}
}

type EthereumAuthParams struct {
	Version  uint  `json:"version"`
	ChainIDs []int `json:"chain_ids"`
}

func (p EthereumAuthParams) GetParams() interface{} {
	copyP := p
	copyP.ChainIDs = make([]int, len(p.ChainIDs))
	copy(copyP.ChainIDs, p.ChainIDs)
	return copyP
}

type EthereumAuthConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Version            uint   `yaml:"version"`
	ChainIDs           []int  `yaml:"chain_ids"`
	DeploymentChainIDs string `yaml:"deployment_chain_ids"` // For deployment: use env variable strings to override the chain IDs.
	EnableAuthz        bool   `yaml:"enable_authz"`         // Flag to enable / disable authorization during development
}

type PublicKeyAuthentication struct {
	Ethereum EthereumAuthConfig `yaml:"ethereum"`
}

func (pk *PublicKeyAuthentication) Enabled() bool {
	return pk.Ethereum.Enabled
}

func (pk *PublicKeyAuthentication) GetPublicKeyRegistrationFlows() []authtypes.Flow {
	var flows []authtypes.Flow
	if pk.Ethereum.Enabled {
		flows = append(flows, authtypes.Flow{Stages: []authtypes.LoginType{authtypes.LoginTypePublicKeyEthereum}})
	}

	return flows
}

func (pk *PublicKeyAuthentication) GetPublicKeyRegistrationParams() map[string]interface{} {
	params := make(map[string]interface{})
	if pk.Ethereum.Enabled {
		p := EthereumAuthParams{
			Version:  pk.Ethereum.Version,
			ChainIDs: pk.Ethereum.ChainIDs,
		}
		params[authtypes.LoginTypePublicKeyEthereum] = p
	}

	return params
}
