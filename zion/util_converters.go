package zion

import (
	"github.com/ethereum/go-ethereum/crypto"
)

func NetworkIdToHash(networkId string) [32]byte {
	hash := crypto.Keccak256Hash([]byte(networkId))
	return sliceBytesToBytes32(hash.Bytes())
}

func sliceBytesToBytes32(bytes []byte) [32]byte {
	bytes32 := [32]byte{}
	for i := 0; i < 32; i++ {
		bytes32[i] = bytes[i]
	}
	return bytes32
}
