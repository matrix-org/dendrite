package zion

import (
	"github.com/ethereum/go-ethereum/ethclient"
)

func GetEthClient(networkUrl string) (*ethclient.Client, error) {
	client, err := ethclient.Dial(networkUrl)
	if err != nil {
		return nil, err
	}

	return client, nil
}
