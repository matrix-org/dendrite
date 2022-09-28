package zion

import (
	"regexp"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

var regexpMatrixId = regexp.MustCompile(`^@eip155=3a(?P<ChainId>[0-9]+)=3a(?P<LocalPart>0x[0-9a-fA-F]+):(?P<HomeServer>.*)$`)
var chainIdIndex = regexpMatrixId.SubexpIndex("ChainId")
var localPartIndex = regexpMatrixId.SubexpIndex("LocalPart")

//var homeServerIndex = regexpMatrixId.SubexpIndex("HomeServer")

type UserIdentifier struct {
	accountAddress common.Address
	chainId        int
}

func CreateUserIdentifier(matrixUserId string) UserIdentifier {
	matches := regexpMatrixId.FindStringSubmatch(matrixUserId)
	accountAddress := ""
	chainId := -1

	if chainIdIndex < len(matches) {
		chainId, _ = strconv.Atoi(matches[chainIdIndex])
	}

	if localPartIndex < len(matches) {
		accountAddress = matches[localPartIndex]
	}

	return UserIdentifier{
		accountAddress: common.HexToAddress(accountAddress),
		chainId:        chainId,
	}
}
