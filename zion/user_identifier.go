package zion

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

var regexpMatrixId = regexp.MustCompile(`^@eip155=3a(?P<ChainId>[0-9]+)=3a(?P<Address>0x[0-9a-fA-F]+):(?P<HomeServer>.*)$`)
var chainIdIndex = regexpMatrixId.SubexpIndex("ChainId")
var addressIndex = regexpMatrixId.SubexpIndex("Address")

//var homeServerIndex = regexpMatrixId.SubexpIndex("HomeServer")

type UserIdentifier struct {
	AccountAddress common.Address
	ChainId        int
	MatrixUserId   string
	LocalPart      string
}

func CreateUserIdentifier(matrixUserId string) UserIdentifier {
	matches := regexpMatrixId.FindStringSubmatch(matrixUserId)
	address := ""
	chainId := -1
	localPart := ""

	if chainIdIndex < len(matches) {
		chainId, _ = strconv.Atoi(matches[chainIdIndex])
	}

	if addressIndex < len(matches) {
		address = matches[addressIndex]
		localPart = fmt.Sprintf("@eip155=3a%d=3a%s", chainId, address)
	}

	return UserIdentifier{
		AccountAddress: common.HexToAddress(address),
		ChainId:        chainId,
		MatrixUserId:   matrixUserId,
		LocalPart:      localPart,
	}
}
