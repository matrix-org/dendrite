package config

import (
	"encoding/base64"
	"math/rand"
	"regexp"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

var nonceByteLength = 32
var minNonceCharacterLength = 8

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
	copyP.Nonce = newNonce()
	return copyP
}

func (p EthereumAuthParams) GetNonce() string {
	return p.Nonce
}

type ethereumAuthConfig struct {
	Enabled  bool  `yaml:"enabled"`
	Version  uint  `yaml:"version"`
	ChainIDs []int `yaml:"chain_ids"`
}

type publicKeyAuthentication struct {
	Ethereum ethereumAuthConfig `yaml:"ethereum"`
}

func (pk *publicKeyAuthentication) Enabled() bool {
	return pk.Ethereum.Enabled
}

func (pk *publicKeyAuthentication) GetPublicKeyRegistrationFlows() []authtypes.Flow {
	var flows []authtypes.Flow
	if pk.Ethereum.Enabled {
		flows = append(flows, authtypes.Flow{Stages: []authtypes.LoginType{authtypes.LoginTypePublicKeyEthereum}})
	}

	return flows
}

func (pk *publicKeyAuthentication) GetPublicKeyRegistrationParams() map[string]interface{} {
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

var regexpNotAlphaDigit = regexp.MustCompile("[^a-zA-Z0-9]+")

func newNonce() string {
	nonce := ""

	for len(nonce) < minNonceCharacterLength {
		b := make([]byte, nonceByteLength)
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		// url-safe no padding
		nonce = base64.RawURLEncoding.EncodeToString(b)
		// Remove any non alphanumeric or digit to comply with spec EIP-4361
		// nonce grammar.
		nonce = regexpNotAlphaDigit.ReplaceAllString(nonce, "")
	}

	return nonce
}
