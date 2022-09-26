package web3

import (
	"github.com/ethereum/go-ethereum/ethclient"
)

func GetEthClient(web3ProviderUrl string) (*ethclient.Client, error) {
	client, err := ethclient.Dial(web3ProviderUrl)
	if err != nil {
		return nil, err
	}

	return client, nil
}
