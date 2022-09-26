package web3

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateTransactionSignerArgs struct {
	PrivateKey string
	ChainId    int64
	Client     *ethclient.Client
	GasValue   int64 // in wei
	GasLimit   int64 // in units
}

func CreateTransactionSigner(args CreateTransactionSignerArgs) (*bind.TransactOpts, error) {
	privateKey, err := crypto.HexToECDSA(args.PrivateKey)
	if err != nil {
		return nil, err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("cannot create public key ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	nonce, err := args.Client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return nil, err
	}

	gasPrice, err := args.Client.SuggestGasPrice((context.Background()))
	if err != nil {
		return nil, err
	}

	signer, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(args.ChainId))
	if err != nil {
		return nil, err
	}

	signer.Nonce = big.NewInt(int64(nonce))
	signer.Value = big.NewInt(args.GasValue)
	signer.GasLimit = uint64(args.GasLimit)
	signer.GasPrice = gasPrice

	fmt.Printf("{ nonce: %d, value: %d, gasLimit: %d, gasPrice: %d }\n",
		signer.Nonce,
		signer.Value,
		signer.GasLimit,
		signer.GasPrice,
	)

	return signer, nil
}
