package zion

import (
	_ "embed"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
	"github.com/matrix-org/dendrite/authorization"
	log "github.com/sirupsen/logrus"
)

const (
	localhostEndpointUrl = "LOCALHOST_ENDPOINT" // .env
	goerliEndpointUrl    = "GOERLI_ENDPOINT"    // .env
)

//go:embed contracts/localhost/addresses/space-manager.json
var localhostJson []byte

//go:embed contracts/goerli/addresses/space-manager.json
var goerliJson []byte

type ZionAuthorization struct {
	spaceManagerLocalhost *ZionSpaceManagerLocalhost
	spaceManagerGoerli    *ZionSpaceManagerGoerli
}

func NewZionAuthorization() (authorization.Authorization, error) {
	err := godotenv.Load(".env")
	if err != nil {
		log.Errorln("error loading .env file", err)
	}

	var auth ZionAuthorization

	localhost, err := newZionSpaceManagerLocalhost(os.Getenv(localhostEndpointUrl))
	if err != nil {
		log.Errorln("error instantiating ZionSpaceManagerLocalhost", err)
	}
	auth.spaceManagerLocalhost = localhost

	goerli, err := newZionSpaceManagerGoerli(os.Getenv(goerliEndpointUrl))
	if err != nil {
		log.Errorln("error instantiating ZionSpaceManagerGoerli", err)
	}
	auth.spaceManagerGoerli = goerli

	return &auth, nil
}

func (za *ZionAuthorization) IsAllowed(args authorization.AuthorizationArgs) (bool, error) {
	userIdentifier := CreateUserIdentifier(args.UserId)
	permission := DataTypesPermission{
		Name: args.Permission,
	}

	switch userIdentifier.chainId {
	case 1337, 31337:
		return za.IsAllowedLocalhost(args.RoomId, userIdentifier.accountAddress, permission)
	case 5:
		return za.IsAllowedGoerli(args.RoomId, userIdentifier.accountAddress, permission)
	default:
		log.Errorf("Unsupported chain id: %d\n", userIdentifier.chainId)
	}

	return false, nil
}

func (za *ZionAuthorization) IsAllowedLocalhost(roomId string, user common.Address, permission DataTypesPermission) (bool, error) {
	if za.spaceManagerLocalhost != nil {
		spaceId, err := za.spaceManagerLocalhost.GetSpaceIdByNetworkId(nil, roomId)
		if err != nil {
			return false, err
		}

		isEntitled, err := za.spaceManagerLocalhost.IsEntitled(
			nil,
			spaceId,
			big.NewInt(0),
			user,
			permission,
		)

		if err != nil {
			return false, err
		}

		return isEntitled, nil
	}

	return false, nil
}

func (za *ZionAuthorization) IsAllowedGoerli(roomId string, user common.Address, permission DataTypesPermission) (bool, error) {
	if za.spaceManagerGoerli != nil {
		spaceId, err := za.spaceManagerGoerli.GetSpaceIdByNetworkId(nil, roomId)
		if err != nil {
			return false, err
		}

		isEntitled, err := za.spaceManagerGoerli.IsEntitled(
			nil,
			spaceId,
			big.NewInt(0),
			user,
			permission,
		)

		if err != nil {
			return false, err
		}

		return isEntitled, nil
	}

	return false, nil
}

func newZionSpaceManagerLocalhost(endpointUrl string) (*ZionSpaceManagerLocalhost, error) {
	addresses, err := loadSpaceManagerAddresses(localhostJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress(addresses.Spacemanager)

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := NewZionSpaceManagerLocalhost(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}

func newZionSpaceManagerGoerli(endpointUrl string) (*ZionSpaceManagerGoerli, error) {
	addresses, err := loadSpaceManagerAddresses(goerliJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress((addresses.Spacemanager))

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := NewZionSpaceManagerGoerli(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}
