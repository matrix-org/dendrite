package zion

import (
	_ "embed"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
	"github.com/matrix-org/dendrite/authorization"
	zion_goerli "github.com/matrix-org/dendrite/zion/contracts/goerli/zion_goerli"
	zion_localhost "github.com/matrix-org/dendrite/zion/contracts/localhost/zion_localhost"
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
	spaceManagerLocalhost *zion_localhost.ZionSpaceManagerLocalhost
	spaceManagerGoerli    *zion_goerli.ZionSpaceManagerGoerli
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

	switch userIdentifier.chainId {
	case 1337, 31337:
		return za.IsAllowedLocalhost(args.RoomId, userIdentifier.accountAddress, args.Permission)
	case 5:
		return za.IsAllowedGoerli(args.RoomId, userIdentifier.accountAddress, args.Permission)
	default:
		log.Errorf("Unsupported chain id: %d\n", userIdentifier.chainId)
	}

	return false, nil
}

func (za *ZionAuthorization) IsAllowedLocalhost(roomId string, user common.Address, permission authorization.Permission) (bool, error) {
	if za.spaceManagerLocalhost != nil {
		permission := zion_localhost.DataTypesPermission{
			Name: permission.String(),
		}

		addr := user.Hex()
		_ = addr

		isEntitled, err := za.spaceManagerLocalhost.IsEntitled(
			nil,
			roomId,
			"", // todo: Support channelId
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

func (za *ZionAuthorization) IsAllowedGoerli(roomId string, user common.Address, permission authorization.Permission) (bool, error) {
	if za.spaceManagerGoerli != nil {
		permission := zion_goerli.DataTypesPermission{
			Name: permission.String(),
		}

		isEntitled, err := za.spaceManagerGoerli.IsEntitled(
			nil,
			roomId,
			"", // todo: support channelId
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

func newZionSpaceManagerLocalhost(endpointUrl string) (*zion_localhost.ZionSpaceManagerLocalhost, error) {
	addresses, err := loadSpaceManagerAddresses(localhostJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress(addresses.Spacemanager)

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := zion_localhost.NewZionSpaceManagerLocalhost(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}

func newZionSpaceManagerGoerli(endpointUrl string) (*zion_goerli.ZionSpaceManagerGoerli, error) {
	addresses, err := loadSpaceManagerAddresses(goerliJson)
	if err != nil {
		return nil, err
	}

	address := common.HexToAddress((addresses.Spacemanager))

	client, err := GetEthClient(endpointUrl)
	if err != nil {
		return nil, err
	}

	spaceManager, err := zion_goerli.NewZionSpaceManagerGoerli(address, client)
	if err != nil {
		return nil, err
	}

	return spaceManager, nil
}
