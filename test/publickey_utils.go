// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strings"

	// This is to silence the conflict about which chainhash is used
	_ "github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spruceid/siwe-go"
)

const EthereumTestNetworkId = 4 // Rinkeby test network ID
const TestServerName = "localhost"

type EthereumTestWallet struct {
	Eip155UserId  string
	PublicAddress string
	PrivateKey    *ecdsa.PrivateKey
}

// https://goethereumbook.org/wallet-generate/
func CreateTestAccount() (*EthereumTestWallet, error) {
	// Create a new public / private key pair.
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	// Get the public key
	publicKey := privateKey.Public()

	// Transform public key to the Ethereum address
	publicKeyEcdsa, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("error casting public key to ECDSA")
	}

	address := crypto.PubkeyToAddress(*publicKeyEcdsa).Hex()
	eip155UserId := fmt.Sprintf("eip155=3a%d=3a%s", EthereumTestNetworkId, address)

	return &EthereumTestWallet{
			PublicAddress: address,
			PrivateKey:    privateKey,
			Eip155UserId:  eip155UserId,
		},
		nil
}

func CreateEip4361TestMessage(
	publicAddress string,
) (*siwe.Message, error) {
	options := make(map[string]interface{})
	options["chainId"] = 4 // Rinkeby test network
	options["statement"] = "This is a test statement"
	message, err := siwe.InitMessage(
		TestServerName,
		publicAddress,
		"https://localhost/login",
		siwe.GenerateNonce(),
		options,
	)

	if err != nil {
		return nil, err
	}

	return message, nil
}

func FromEip4361MessageToString(message *siwe.Message) string {
	// Escape the formatting characters to
	// prevent unmarshal exceptions.
	str := strings.ReplaceAll(message.String(), "\n", "\\n")
	str = strings.ReplaceAll(str, "\t", "\\t")
	return str
}

// https://goethereumbook.org/signature-generate/
func SignMessage(message string, privateKey *ecdsa.PrivateKey) (string, error) {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	data := []byte(msg)
	hash := crypto.Keccak256Hash(data)

	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return "", err
	}

	// https://github.com/ethereum/go-ethereum/blob/55599ee95d4151a2502465e0afc7c47bd1acba77/internal/ethapi/api.go#L442
	signature[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	return hexutil.Encode(signature), nil
}
