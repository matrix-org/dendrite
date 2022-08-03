package config

import (
	"math/rand"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

var nonceLength = 32

type AuthParams interface {
	GetParams() interface{}
	GetNonce() string
}

type EthereumAuthParams struct {
	Version  uint   `json:"version"`
	ChainIDs []int  `json:"chain_ids"`
	Nonce    string `json:"nonce"`
}

func (p EthereumAuthParams) GetParams() interface{} {
	copyP := p
	copyP.ChainIDs = make([]int, len(p.ChainIDs))
	copy(copyP.ChainIDs, p.ChainIDs)
	copyP.Nonce = newNonce(nonceLength)
	return copyP
}

func (p EthereumAuthParams) GetNonce() string {
	return p.Nonce
}

type EthereumAuthConfig struct {
	Enabled  bool  `yaml:"enabled"`
	Version  uint  `yaml:"version"`
	ChainIDs []int `yaml:"chain_ids"`
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
			Nonce:    "",
		}
		params[authtypes.LoginTypePublicKeyEthereum] = p
	}

	return params
}

const lettersAndNumbers = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func newNonce(n int) string {
	nonce := make([]byte, n)
	rand.Seed(time.Now().UnixNano())

	for i := range nonce {
		nonce[i] = lettersAndNumbers[rand.Int63()%int64(len(lettersAndNumbers))]
	}

	return string(nonce)
}
