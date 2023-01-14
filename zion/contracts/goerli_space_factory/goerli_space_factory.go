// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package goerli_space_factory

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// DataTypesCreateSpaceExtraEntitlements is an auto generated low-level Go binding around an user-defined struct.
type DataTypesCreateSpaceExtraEntitlements struct {
	RoleName    string
	Permissions []string
	Tokens      []DataTypesExternalToken
	Users       []common.Address
}

// DataTypesExternalToken is an auto generated low-level Go binding around an user-defined struct.
type DataTypesExternalToken struct {
	ContractAddress common.Address
	Quantity        *big.Int
	IsSingleToken   bool
	TokenIds        []*big.Int
}

// GoerliSpaceFactoryMetaData contains all meta data concerning the GoerliSpaceFactory contract.
var GoerliSpaceFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameContainsInvalidCharacters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameLengthInvalid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"PermissionAlreadyExists\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"SpaceAlreadyRegistered\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"previousAdmin\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"newAdmin\",\"type\":\"address\"}],\"name\":\"AdminChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"beacon\",\"type\":\"address\"}],\"name\":\"BeaconUpgraded\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"version\",\"type\":\"uint8\"}],\"name\":\"Initialized\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"implementation\",\"type\":\"address\"}],\"name\":\"Upgraded\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"SPACE_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SPACE_TOKEN_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"TOKEN_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"USER_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"}],\"name\":\"addOwnerPermissions\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"spaceNetworkId\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"spaceMetadata\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"_everyonePermissions\",\"type\":\"string[]\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"roleName\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"permissions\",\"type\":\"string[]\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"quantity\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"isSingleToken\",\"type\":\"bool\"},{\"internalType\":\"uint256[]\",\"name\":\"tokenIds\",\"type\":\"uint256[]\"}],\"internalType\":\"structDataTypes.ExternalToken[]\",\"name\":\"tokens\",\"type\":\"tuple[]\"},{\"internalType\":\"address[]\",\"name\":\"users\",\"type\":\"address[]\"}],\"internalType\":\"structDataTypes.CreateSpaceExtraEntitlements\",\"name\":\"_extraEntitlements\",\"type\":\"tuple\"}],\"name\":\"createSpace\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"_spaceAddress\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getOwnerPermissions\",\"outputs\":[{\"internalType\":\"string[]\",\"name\":\"\",\"type\":\"string[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"spaceNetworkId\",\"type\":\"string\"}],\"name\":\"getTokenIdByNetworkId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_space\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_tokenEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_userEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_spaceToken\",\"type\":\"address\"},{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"}],\"name\":\"initialize\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"ownerPermissions\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"proxiableUUID\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"spaceByHash\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"tokenByHash\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_space\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_tokenEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_userEntitlement\",\"type\":\"address\"}],\"name\":\"updateImplementations\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"}],\"name\":\"upgradeTo\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"upgradeToAndCall\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"}]",
}

// GoerliSpaceFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use GoerliSpaceFactoryMetaData.ABI instead.
var GoerliSpaceFactoryABI = GoerliSpaceFactoryMetaData.ABI

// GoerliSpaceFactory is an auto generated Go binding around an Ethereum contract.
type GoerliSpaceFactory struct {
	GoerliSpaceFactoryCaller     // Read-only binding to the contract
	GoerliSpaceFactoryTransactor // Write-only binding to the contract
	GoerliSpaceFactoryFilterer   // Log filterer for contract events
}

// GoerliSpaceFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type GoerliSpaceFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GoerliSpaceFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type GoerliSpaceFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GoerliSpaceFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type GoerliSpaceFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GoerliSpaceFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type GoerliSpaceFactorySession struct {
	Contract     *GoerliSpaceFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// GoerliSpaceFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type GoerliSpaceFactoryCallerSession struct {
	Contract *GoerliSpaceFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// GoerliSpaceFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type GoerliSpaceFactoryTransactorSession struct {
	Contract     *GoerliSpaceFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// GoerliSpaceFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type GoerliSpaceFactoryRaw struct {
	Contract *GoerliSpaceFactory // Generic contract binding to access the raw methods on
}

// GoerliSpaceFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type GoerliSpaceFactoryCallerRaw struct {
	Contract *GoerliSpaceFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// GoerliSpaceFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type GoerliSpaceFactoryTransactorRaw struct {
	Contract *GoerliSpaceFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewGoerliSpaceFactory creates a new instance of GoerliSpaceFactory, bound to a specific deployed contract.
func NewGoerliSpaceFactory(address common.Address, backend bind.ContractBackend) (*GoerliSpaceFactory, error) {
	contract, err := bindGoerliSpaceFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactory{GoerliSpaceFactoryCaller: GoerliSpaceFactoryCaller{contract: contract}, GoerliSpaceFactoryTransactor: GoerliSpaceFactoryTransactor{contract: contract}, GoerliSpaceFactoryFilterer: GoerliSpaceFactoryFilterer{contract: contract}}, nil
}

// NewGoerliSpaceFactoryCaller creates a new read-only instance of GoerliSpaceFactory, bound to a specific deployed contract.
func NewGoerliSpaceFactoryCaller(address common.Address, caller bind.ContractCaller) (*GoerliSpaceFactoryCaller, error) {
	contract, err := bindGoerliSpaceFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryCaller{contract: contract}, nil
}

// NewGoerliSpaceFactoryTransactor creates a new write-only instance of GoerliSpaceFactory, bound to a specific deployed contract.
func NewGoerliSpaceFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*GoerliSpaceFactoryTransactor, error) {
	contract, err := bindGoerliSpaceFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryTransactor{contract: contract}, nil
}

// NewGoerliSpaceFactoryFilterer creates a new log filterer instance of GoerliSpaceFactory, bound to a specific deployed contract.
func NewGoerliSpaceFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*GoerliSpaceFactoryFilterer, error) {
	contract, err := bindGoerliSpaceFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryFilterer{contract: contract}, nil
}

// bindGoerliSpaceFactory binds a generic wrapper to an already deployed contract.
func bindGoerliSpaceFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(GoerliSpaceFactoryABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_GoerliSpaceFactory *GoerliSpaceFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _GoerliSpaceFactory.Contract.GoerliSpaceFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_GoerliSpaceFactory *GoerliSpaceFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.GoerliSpaceFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_GoerliSpaceFactory *GoerliSpaceFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.GoerliSpaceFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _GoerliSpaceFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.contract.Transact(opts, method, params...)
}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) SPACEIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "SPACE_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) SPACEIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SPACEIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) SPACEIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SPACEIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) SPACETOKENADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "SPACE_TOKEN_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) SPACETOKENADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SPACETOKENADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) SPACETOKENADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SPACETOKENADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) TOKENIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "TOKEN_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) TOKENIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.TOKENIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) TOKENIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.TOKENIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) USERIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "USER_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) USERIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.USERIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) USERIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.USERIMPLEMENTATIONADDRESS(&_GoerliSpaceFactory.CallOpts)
}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) GetOwnerPermissions(opts *bind.CallOpts) ([]string, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "getOwnerPermissions")

	if err != nil {
		return *new([]string), err
	}

	out0 := *abi.ConvertType(out[0], new([]string)).(*[]string)

	return out0, err

}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) GetOwnerPermissions() ([]string, error) {
	return _GoerliSpaceFactory.Contract.GetOwnerPermissions(&_GoerliSpaceFactory.CallOpts)
}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) GetOwnerPermissions() ([]string, error) {
	return _GoerliSpaceFactory.Contract.GetOwnerPermissions(&_GoerliSpaceFactory.CallOpts)
}

// GetTokenIdByNetworkId is a free data retrieval call binding the contract method 0x8a9ef426.
//
// Solidity: function getTokenIdByNetworkId(string spaceNetworkId) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) GetTokenIdByNetworkId(opts *bind.CallOpts, spaceNetworkId string) (*big.Int, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "getTokenIdByNetworkId", spaceNetworkId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetTokenIdByNetworkId is a free data retrieval call binding the contract method 0x8a9ef426.
//
// Solidity: function getTokenIdByNetworkId(string spaceNetworkId) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) GetTokenIdByNetworkId(spaceNetworkId string) (*big.Int, error) {
	return _GoerliSpaceFactory.Contract.GetTokenIdByNetworkId(&_GoerliSpaceFactory.CallOpts, spaceNetworkId)
}

// GetTokenIdByNetworkId is a free data retrieval call binding the contract method 0x8a9ef426.
//
// Solidity: function getTokenIdByNetworkId(string spaceNetworkId) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) GetTokenIdByNetworkId(spaceNetworkId string) (*big.Int, error) {
	return _GoerliSpaceFactory.Contract.GetTokenIdByNetworkId(&_GoerliSpaceFactory.CallOpts, spaceNetworkId)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) Owner() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.Owner(&_GoerliSpaceFactory.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) Owner() (common.Address, error) {
	return _GoerliSpaceFactory.Contract.Owner(&_GoerliSpaceFactory.CallOpts)
}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) OwnerPermissions(opts *bind.CallOpts, arg0 *big.Int) (string, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "ownerPermissions", arg0)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) OwnerPermissions(arg0 *big.Int) (string, error) {
	return _GoerliSpaceFactory.Contract.OwnerPermissions(&_GoerliSpaceFactory.CallOpts, arg0)
}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) OwnerPermissions(arg0 *big.Int) (string, error) {
	return _GoerliSpaceFactory.Contract.OwnerPermissions(&_GoerliSpaceFactory.CallOpts, arg0)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) ProxiableUUID(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "proxiableUUID")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) ProxiableUUID() ([32]byte, error) {
	return _GoerliSpaceFactory.Contract.ProxiableUUID(&_GoerliSpaceFactory.CallOpts)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) ProxiableUUID() ([32]byte, error) {
	return _GoerliSpaceFactory.Contract.ProxiableUUID(&_GoerliSpaceFactory.CallOpts)
}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) SpaceByHash(opts *bind.CallOpts, arg0 [32]byte) (common.Address, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "spaceByHash", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) SpaceByHash(arg0 [32]byte) (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SpaceByHash(&_GoerliSpaceFactory.CallOpts, arg0)
}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) SpaceByHash(arg0 [32]byte) (common.Address, error) {
	return _GoerliSpaceFactory.Contract.SpaceByHash(&_GoerliSpaceFactory.CallOpts, arg0)
}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCaller) TokenByHash(opts *bind.CallOpts, arg0 [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _GoerliSpaceFactory.contract.Call(opts, &out, "tokenByHash", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) TokenByHash(arg0 [32]byte) (*big.Int, error) {
	return _GoerliSpaceFactory.Contract.TokenByHash(&_GoerliSpaceFactory.CallOpts, arg0)
}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_GoerliSpaceFactory *GoerliSpaceFactoryCallerSession) TokenByHash(arg0 [32]byte) (*big.Int, error) {
	return _GoerliSpaceFactory.Contract.TokenByHash(&_GoerliSpaceFactory.CallOpts, arg0)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) AddOwnerPermissions(opts *bind.TransactOpts, _permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "addOwnerPermissions", _permissions)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) AddOwnerPermissions(_permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.AddOwnerPermissions(&_GoerliSpaceFactory.TransactOpts, _permissions)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) AddOwnerPermissions(_permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.AddOwnerPermissions(&_GoerliSpaceFactory.TransactOpts, _permissions)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) CreateSpace(opts *bind.TransactOpts, spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "createSpace", spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) CreateSpace(spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.CreateSpace(&_GoerliSpaceFactory.TransactOpts, spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) CreateSpace(spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.CreateSpace(&_GoerliSpaceFactory.TransactOpts, spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) Initialize(opts *bind.TransactOpts, _space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "initialize", _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) Initialize(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.Initialize(&_GoerliSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) Initialize(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.Initialize(&_GoerliSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) RenounceOwnership() (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.RenounceOwnership(&_GoerliSpaceFactory.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.RenounceOwnership(&_GoerliSpaceFactory.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.TransferOwnership(&_GoerliSpaceFactory.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.TransferOwnership(&_GoerliSpaceFactory.TransactOpts, newOwner)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) UpdateImplementations(opts *bind.TransactOpts, _space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "updateImplementations", _space, _tokenEntitlement, _userEntitlement)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) UpdateImplementations(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpdateImplementations(&_GoerliSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) UpdateImplementations(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpdateImplementations(&_GoerliSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) UpgradeTo(opts *bind.TransactOpts, newImplementation common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "upgradeTo", newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpgradeTo(&_GoerliSpaceFactory.TransactOpts, newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpgradeTo(&_GoerliSpaceFactory.TransactOpts, newImplementation)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactor) UpgradeToAndCall(opts *bind.TransactOpts, newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _GoerliSpaceFactory.contract.Transact(opts, "upgradeToAndCall", newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_GoerliSpaceFactory *GoerliSpaceFactorySession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpgradeToAndCall(&_GoerliSpaceFactory.TransactOpts, newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_GoerliSpaceFactory *GoerliSpaceFactoryTransactorSession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _GoerliSpaceFactory.Contract.UpgradeToAndCall(&_GoerliSpaceFactory.TransactOpts, newImplementation, data)
}

// GoerliSpaceFactoryAdminChangedIterator is returned from FilterAdminChanged and is used to iterate over the raw logs and unpacked data for AdminChanged events raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryAdminChangedIterator struct {
	Event *GoerliSpaceFactoryAdminChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GoerliSpaceFactoryAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GoerliSpaceFactoryAdminChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GoerliSpaceFactoryAdminChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GoerliSpaceFactoryAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GoerliSpaceFactoryAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GoerliSpaceFactoryAdminChanged represents a AdminChanged event raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryAdminChanged struct {
	PreviousAdmin common.Address
	NewAdmin      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterAdminChanged is a free log retrieval operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) FilterAdminChanged(opts *bind.FilterOpts) (*GoerliSpaceFactoryAdminChangedIterator, error) {

	logs, sub, err := _GoerliSpaceFactory.contract.FilterLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryAdminChangedIterator{contract: _GoerliSpaceFactory.contract, event: "AdminChanged", logs: logs, sub: sub}, nil
}

// WatchAdminChanged is a free log subscription operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) WatchAdminChanged(opts *bind.WatchOpts, sink chan<- *GoerliSpaceFactoryAdminChanged) (event.Subscription, error) {

	logs, sub, err := _GoerliSpaceFactory.contract.WatchLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GoerliSpaceFactoryAdminChanged)
				if err := _GoerliSpaceFactory.contract.UnpackLog(event, "AdminChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAdminChanged is a log parse operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) ParseAdminChanged(log types.Log) (*GoerliSpaceFactoryAdminChanged, error) {
	event := new(GoerliSpaceFactoryAdminChanged)
	if err := _GoerliSpaceFactory.contract.UnpackLog(event, "AdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GoerliSpaceFactoryBeaconUpgradedIterator is returned from FilterBeaconUpgraded and is used to iterate over the raw logs and unpacked data for BeaconUpgraded events raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryBeaconUpgradedIterator struct {
	Event *GoerliSpaceFactoryBeaconUpgraded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GoerliSpaceFactoryBeaconUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GoerliSpaceFactoryBeaconUpgraded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GoerliSpaceFactoryBeaconUpgraded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GoerliSpaceFactoryBeaconUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GoerliSpaceFactoryBeaconUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GoerliSpaceFactoryBeaconUpgraded represents a BeaconUpgraded event raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryBeaconUpgraded struct {
	Beacon common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterBeaconUpgraded is a free log retrieval operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) FilterBeaconUpgraded(opts *bind.FilterOpts, beacon []common.Address) (*GoerliSpaceFactoryBeaconUpgradedIterator, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.FilterLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryBeaconUpgradedIterator{contract: _GoerliSpaceFactory.contract, event: "BeaconUpgraded", logs: logs, sub: sub}, nil
}

// WatchBeaconUpgraded is a free log subscription operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) WatchBeaconUpgraded(opts *bind.WatchOpts, sink chan<- *GoerliSpaceFactoryBeaconUpgraded, beacon []common.Address) (event.Subscription, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.WatchLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GoerliSpaceFactoryBeaconUpgraded)
				if err := _GoerliSpaceFactory.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseBeaconUpgraded is a log parse operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) ParseBeaconUpgraded(log types.Log) (*GoerliSpaceFactoryBeaconUpgraded, error) {
	event := new(GoerliSpaceFactoryBeaconUpgraded)
	if err := _GoerliSpaceFactory.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GoerliSpaceFactoryInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryInitializedIterator struct {
	Event *GoerliSpaceFactoryInitialized // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GoerliSpaceFactoryInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GoerliSpaceFactoryInitialized)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GoerliSpaceFactoryInitialized)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GoerliSpaceFactoryInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GoerliSpaceFactoryInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GoerliSpaceFactoryInitialized represents a Initialized event raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryInitialized struct {
	Version uint8
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) FilterInitialized(opts *bind.FilterOpts) (*GoerliSpaceFactoryInitializedIterator, error) {

	logs, sub, err := _GoerliSpaceFactory.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryInitializedIterator{contract: _GoerliSpaceFactory.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *GoerliSpaceFactoryInitialized) (event.Subscription, error) {

	logs, sub, err := _GoerliSpaceFactory.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GoerliSpaceFactoryInitialized)
				if err := _GoerliSpaceFactory.contract.UnpackLog(event, "Initialized", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInitialized is a log parse operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) ParseInitialized(log types.Log) (*GoerliSpaceFactoryInitialized, error) {
	event := new(GoerliSpaceFactoryInitialized)
	if err := _GoerliSpaceFactory.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GoerliSpaceFactoryOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryOwnershipTransferredIterator struct {
	Event *GoerliSpaceFactoryOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GoerliSpaceFactoryOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GoerliSpaceFactoryOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GoerliSpaceFactoryOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GoerliSpaceFactoryOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GoerliSpaceFactoryOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GoerliSpaceFactoryOwnershipTransferred represents a OwnershipTransferred event raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*GoerliSpaceFactoryOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryOwnershipTransferredIterator{contract: _GoerliSpaceFactory.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *GoerliSpaceFactoryOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GoerliSpaceFactoryOwnershipTransferred)
				if err := _GoerliSpaceFactory.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) ParseOwnershipTransferred(log types.Log) (*GoerliSpaceFactoryOwnershipTransferred, error) {
	event := new(GoerliSpaceFactoryOwnershipTransferred)
	if err := _GoerliSpaceFactory.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// GoerliSpaceFactoryUpgradedIterator is returned from FilterUpgraded and is used to iterate over the raw logs and unpacked data for Upgraded events raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryUpgradedIterator struct {
	Event *GoerliSpaceFactoryUpgraded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GoerliSpaceFactoryUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GoerliSpaceFactoryUpgraded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GoerliSpaceFactoryUpgraded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GoerliSpaceFactoryUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GoerliSpaceFactoryUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GoerliSpaceFactoryUpgraded represents a Upgraded event raised by the GoerliSpaceFactory contract.
type GoerliSpaceFactoryUpgraded struct {
	Implementation common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgraded is a free log retrieval operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) FilterUpgraded(opts *bind.FilterOpts, implementation []common.Address) (*GoerliSpaceFactoryUpgradedIterator, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.FilterLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return &GoerliSpaceFactoryUpgradedIterator{contract: _GoerliSpaceFactory.contract, event: "Upgraded", logs: logs, sub: sub}, nil
}

// WatchUpgraded is a free log subscription operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) WatchUpgraded(opts *bind.WatchOpts, sink chan<- *GoerliSpaceFactoryUpgraded, implementation []common.Address) (event.Subscription, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _GoerliSpaceFactory.contract.WatchLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GoerliSpaceFactoryUpgraded)
				if err := _GoerliSpaceFactory.contract.UnpackLog(event, "Upgraded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpgraded is a log parse operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_GoerliSpaceFactory *GoerliSpaceFactoryFilterer) ParseUpgraded(log types.Log) (*GoerliSpaceFactoryUpgraded, error) {
	event := new(GoerliSpaceFactoryUpgraded)
	if err := _GoerliSpaceFactory.contract.UnpackLog(event, "Upgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
