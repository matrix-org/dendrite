// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package localhost_space_factory

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

// LocalhostSpaceFactoryMetaData contains all meta data concerning the LocalhostSpaceFactory contract.
var LocalhostSpaceFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameContainsInvalidCharacters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameLengthInvalid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"PermissionAlreadyExists\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"SpaceAlreadyRegistered\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"previousAdmin\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"newAdmin\",\"type\":\"address\"}],\"name\":\"AdminChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"beacon\",\"type\":\"address\"}],\"name\":\"BeaconUpgraded\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"version\",\"type\":\"uint8\"}],\"name\":\"Initialized\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"implementation\",\"type\":\"address\"}],\"name\":\"Upgraded\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"SPACE_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"SPACE_TOKEN_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"TOKEN_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"USER_IMPLEMENTATION_ADDRESS\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"}],\"name\":\"addOwnerPermissions\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"spaceNetworkId\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"spaceMetadata\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"_everyonePermissions\",\"type\":\"string[]\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"roleName\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"permissions\",\"type\":\"string[]\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"quantity\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"isSingleToken\",\"type\":\"bool\"},{\"internalType\":\"uint256[]\",\"name\":\"tokenIds\",\"type\":\"uint256[]\"}],\"internalType\":\"structDataTypes.ExternalToken[]\",\"name\":\"tokens\",\"type\":\"tuple[]\"},{\"internalType\":\"address[]\",\"name\":\"users\",\"type\":\"address[]\"}],\"internalType\":\"structDataTypes.CreateSpaceExtraEntitlements\",\"name\":\"_extraEntitlements\",\"type\":\"tuple\"}],\"name\":\"createSpace\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"_spaceAddress\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getOwnerPermissions\",\"outputs\":[{\"internalType\":\"string[]\",\"name\":\"\",\"type\":\"string[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_space\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_tokenEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_userEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_spaceToken\",\"type\":\"address\"},{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"}],\"name\":\"initialize\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"ownerPermissions\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"proxiableUUID\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"spaceByHash\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"tokenByHash\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_space\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_tokenEntitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_userEntitlement\",\"type\":\"address\"}],\"name\":\"updateImplementations\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"}],\"name\":\"upgradeTo\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"upgradeToAndCall\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"}]",
}

// LocalhostSpaceFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use LocalhostSpaceFactoryMetaData.ABI instead.
var LocalhostSpaceFactoryABI = LocalhostSpaceFactoryMetaData.ABI

// LocalhostSpaceFactory is an auto generated Go binding around an Ethereum contract.
type LocalhostSpaceFactory struct {
	LocalhostSpaceFactoryCaller     // Read-only binding to the contract
	LocalhostSpaceFactoryTransactor // Write-only binding to the contract
	LocalhostSpaceFactoryFilterer   // Log filterer for contract events
}

// LocalhostSpaceFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type LocalhostSpaceFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type LocalhostSpaceFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type LocalhostSpaceFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type LocalhostSpaceFactorySession struct {
	Contract     *LocalhostSpaceFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts          // Call options to use throughout this session
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// LocalhostSpaceFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type LocalhostSpaceFactoryCallerSession struct {
	Contract *LocalhostSpaceFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                // Call options to use throughout this session
}

// LocalhostSpaceFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type LocalhostSpaceFactoryTransactorSession struct {
	Contract     *LocalhostSpaceFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                // Transaction auth options to use throughout this session
}

// LocalhostSpaceFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type LocalhostSpaceFactoryRaw struct {
	Contract *LocalhostSpaceFactory // Generic contract binding to access the raw methods on
}

// LocalhostSpaceFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type LocalhostSpaceFactoryCallerRaw struct {
	Contract *LocalhostSpaceFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// LocalhostSpaceFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type LocalhostSpaceFactoryTransactorRaw struct {
	Contract *LocalhostSpaceFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewLocalhostSpaceFactory creates a new instance of LocalhostSpaceFactory, bound to a specific deployed contract.
func NewLocalhostSpaceFactory(address common.Address, backend bind.ContractBackend) (*LocalhostSpaceFactory, error) {
	contract, err := bindLocalhostSpaceFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactory{LocalhostSpaceFactoryCaller: LocalhostSpaceFactoryCaller{contract: contract}, LocalhostSpaceFactoryTransactor: LocalhostSpaceFactoryTransactor{contract: contract}, LocalhostSpaceFactoryFilterer: LocalhostSpaceFactoryFilterer{contract: contract}}, nil
}

// NewLocalhostSpaceFactoryCaller creates a new read-only instance of LocalhostSpaceFactory, bound to a specific deployed contract.
func NewLocalhostSpaceFactoryCaller(address common.Address, caller bind.ContractCaller) (*LocalhostSpaceFactoryCaller, error) {
	contract, err := bindLocalhostSpaceFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryCaller{contract: contract}, nil
}

// NewLocalhostSpaceFactoryTransactor creates a new write-only instance of LocalhostSpaceFactory, bound to a specific deployed contract.
func NewLocalhostSpaceFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*LocalhostSpaceFactoryTransactor, error) {
	contract, err := bindLocalhostSpaceFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryTransactor{contract: contract}, nil
}

// NewLocalhostSpaceFactoryFilterer creates a new log filterer instance of LocalhostSpaceFactory, bound to a specific deployed contract.
func NewLocalhostSpaceFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*LocalhostSpaceFactoryFilterer, error) {
	contract, err := bindLocalhostSpaceFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryFilterer{contract: contract}, nil
}

// bindLocalhostSpaceFactory binds a generic wrapper to an already deployed contract.
func bindLocalhostSpaceFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(LocalhostSpaceFactoryABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LocalhostSpaceFactory.Contract.LocalhostSpaceFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.LocalhostSpaceFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.LocalhostSpaceFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LocalhostSpaceFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.contract.Transact(opts, method, params...)
}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) SPACEIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "SPACE_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) SPACEIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SPACEIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// SPACEIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xf21cd401.
//
// Solidity: function SPACE_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) SPACEIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SPACEIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) SPACETOKENADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "SPACE_TOKEN_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) SPACETOKENADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SPACETOKENADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// SPACETOKENADDRESS is a free data retrieval call binding the contract method 0x683c72b6.
//
// Solidity: function SPACE_TOKEN_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) SPACETOKENADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SPACETOKENADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) TOKENIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "TOKEN_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) TOKENIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.TOKENIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// TOKENIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0xcfc27037.
//
// Solidity: function TOKEN_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) TOKENIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.TOKENIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) USERIMPLEMENTATIONADDRESS(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "USER_IMPLEMENTATION_ADDRESS")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) USERIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.USERIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// USERIMPLEMENTATIONADDRESS is a free data retrieval call binding the contract method 0x08bc0b4b.
//
// Solidity: function USER_IMPLEMENTATION_ADDRESS() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) USERIMPLEMENTATIONADDRESS() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.USERIMPLEMENTATIONADDRESS(&_LocalhostSpaceFactory.CallOpts)
}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) GetOwnerPermissions(opts *bind.CallOpts) ([]string, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "getOwnerPermissions")

	if err != nil {
		return *new([]string), err
	}

	out0 := *abi.ConvertType(out[0], new([]string)).(*[]string)

	return out0, err

}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) GetOwnerPermissions() ([]string, error) {
	return _LocalhostSpaceFactory.Contract.GetOwnerPermissions(&_LocalhostSpaceFactory.CallOpts)
}

// GetOwnerPermissions is a free data retrieval call binding the contract method 0xdf2cd9fe.
//
// Solidity: function getOwnerPermissions() view returns(string[])
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) GetOwnerPermissions() ([]string, error) {
	return _LocalhostSpaceFactory.Contract.GetOwnerPermissions(&_LocalhostSpaceFactory.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) Owner() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.Owner(&_LocalhostSpaceFactory.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) Owner() (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.Owner(&_LocalhostSpaceFactory.CallOpts)
}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) OwnerPermissions(opts *bind.CallOpts, arg0 *big.Int) (string, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "ownerPermissions", arg0)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) OwnerPermissions(arg0 *big.Int) (string, error) {
	return _LocalhostSpaceFactory.Contract.OwnerPermissions(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// OwnerPermissions is a free data retrieval call binding the contract method 0xb28032f9.
//
// Solidity: function ownerPermissions(uint256 ) view returns(string)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) OwnerPermissions(arg0 *big.Int) (string, error) {
	return _LocalhostSpaceFactory.Contract.OwnerPermissions(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) ProxiableUUID(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "proxiableUUID")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) ProxiableUUID() ([32]byte, error) {
	return _LocalhostSpaceFactory.Contract.ProxiableUUID(&_LocalhostSpaceFactory.CallOpts)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) ProxiableUUID() ([32]byte, error) {
	return _LocalhostSpaceFactory.Contract.ProxiableUUID(&_LocalhostSpaceFactory.CallOpts)
}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) SpaceByHash(opts *bind.CallOpts, arg0 [32]byte) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "spaceByHash", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) SpaceByHash(arg0 [32]byte) (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SpaceByHash(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// SpaceByHash is a free data retrieval call binding the contract method 0x3312540a.
//
// Solidity: function spaceByHash(bytes32 ) view returns(address)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) SpaceByHash(arg0 [32]byte) (common.Address, error) {
	return _LocalhostSpaceFactory.Contract.SpaceByHash(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCaller) TokenByHash(opts *bind.CallOpts, arg0 [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _LocalhostSpaceFactory.contract.Call(opts, &out, "tokenByHash", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) TokenByHash(arg0 [32]byte) (*big.Int, error) {
	return _LocalhostSpaceFactory.Contract.TokenByHash(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// TokenByHash is a free data retrieval call binding the contract method 0xf3aba305.
//
// Solidity: function tokenByHash(bytes32 ) view returns(uint256)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryCallerSession) TokenByHash(arg0 [32]byte) (*big.Int, error) {
	return _LocalhostSpaceFactory.Contract.TokenByHash(&_LocalhostSpaceFactory.CallOpts, arg0)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) AddOwnerPermissions(opts *bind.TransactOpts, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "addOwnerPermissions", _permissions)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) AddOwnerPermissions(_permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.AddOwnerPermissions(&_LocalhostSpaceFactory.TransactOpts, _permissions)
}

// AddOwnerPermissions is a paid mutator transaction binding the contract method 0xbe8b5967.
//
// Solidity: function addOwnerPermissions(string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) AddOwnerPermissions(_permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.AddOwnerPermissions(&_LocalhostSpaceFactory.TransactOpts, _permissions)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) CreateSpace(opts *bind.TransactOpts, spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "createSpace", spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) CreateSpace(spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.CreateSpace(&_LocalhostSpaceFactory.TransactOpts, spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// CreateSpace is a paid mutator transaction binding the contract method 0xad78faf3.
//
// Solidity: function createSpace(string spaceName, string spaceNetworkId, string spaceMetadata, string[] _everyonePermissions, (string,string[],(address,uint256,bool,uint256[])[],address[]) _extraEntitlements) returns(address _spaceAddress)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) CreateSpace(spaceName string, spaceNetworkId string, spaceMetadata string, _everyonePermissions []string, _extraEntitlements DataTypesCreateSpaceExtraEntitlements) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.CreateSpace(&_LocalhostSpaceFactory.TransactOpts, spaceName, spaceNetworkId, spaceMetadata, _everyonePermissions, _extraEntitlements)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) Initialize(opts *bind.TransactOpts, _space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "initialize", _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) Initialize(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.Initialize(&_LocalhostSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// Initialize is a paid mutator transaction binding the contract method 0x45bfa5b1.
//
// Solidity: function initialize(address _space, address _tokenEntitlement, address _userEntitlement, address _spaceToken, string[] _permissions) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) Initialize(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address, _spaceToken common.Address, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.Initialize(&_LocalhostSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement, _spaceToken, _permissions)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) RenounceOwnership() (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.RenounceOwnership(&_LocalhostSpaceFactory.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.RenounceOwnership(&_LocalhostSpaceFactory.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.TransferOwnership(&_LocalhostSpaceFactory.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.TransferOwnership(&_LocalhostSpaceFactory.TransactOpts, newOwner)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) UpdateImplementations(opts *bind.TransactOpts, _space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "updateImplementations", _space, _tokenEntitlement, _userEntitlement)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) UpdateImplementations(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpdateImplementations(&_LocalhostSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement)
}

// UpdateImplementations is a paid mutator transaction binding the contract method 0xdfc666ff.
//
// Solidity: function updateImplementations(address _space, address _tokenEntitlement, address _userEntitlement) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) UpdateImplementations(_space common.Address, _tokenEntitlement common.Address, _userEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpdateImplementations(&_LocalhostSpaceFactory.TransactOpts, _space, _tokenEntitlement, _userEntitlement)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) UpgradeTo(opts *bind.TransactOpts, newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "upgradeTo", newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpgradeTo(&_LocalhostSpaceFactory.TransactOpts, newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpgradeTo(&_LocalhostSpaceFactory.TransactOpts, newImplementation)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactor) UpgradeToAndCall(opts *bind.TransactOpts, newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.contract.Transact(opts, "upgradeToAndCall", newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactorySession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpgradeToAndCall(&_LocalhostSpaceFactory.TransactOpts, newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryTransactorSession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpaceFactory.Contract.UpgradeToAndCall(&_LocalhostSpaceFactory.TransactOpts, newImplementation, data)
}

// LocalhostSpaceFactoryAdminChangedIterator is returned from FilterAdminChanged and is used to iterate over the raw logs and unpacked data for AdminChanged events raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryAdminChangedIterator struct {
	Event *LocalhostSpaceFactoryAdminChanged // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceFactoryAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceFactoryAdminChanged)
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
		it.Event = new(LocalhostSpaceFactoryAdminChanged)
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
func (it *LocalhostSpaceFactoryAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceFactoryAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceFactoryAdminChanged represents a AdminChanged event raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryAdminChanged struct {
	PreviousAdmin common.Address
	NewAdmin      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterAdminChanged is a free log retrieval operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) FilterAdminChanged(opts *bind.FilterOpts) (*LocalhostSpaceFactoryAdminChangedIterator, error) {

	logs, sub, err := _LocalhostSpaceFactory.contract.FilterLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryAdminChangedIterator{contract: _LocalhostSpaceFactory.contract, event: "AdminChanged", logs: logs, sub: sub}, nil
}

// WatchAdminChanged is a free log subscription operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) WatchAdminChanged(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceFactoryAdminChanged) (event.Subscription, error) {

	logs, sub, err := _LocalhostSpaceFactory.contract.WatchLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceFactoryAdminChanged)
				if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "AdminChanged", log); err != nil {
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
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) ParseAdminChanged(log types.Log) (*LocalhostSpaceFactoryAdminChanged, error) {
	event := new(LocalhostSpaceFactoryAdminChanged)
	if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "AdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceFactoryBeaconUpgradedIterator is returned from FilterBeaconUpgraded and is used to iterate over the raw logs and unpacked data for BeaconUpgraded events raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryBeaconUpgradedIterator struct {
	Event *LocalhostSpaceFactoryBeaconUpgraded // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceFactoryBeaconUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceFactoryBeaconUpgraded)
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
		it.Event = new(LocalhostSpaceFactoryBeaconUpgraded)
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
func (it *LocalhostSpaceFactoryBeaconUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceFactoryBeaconUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceFactoryBeaconUpgraded represents a BeaconUpgraded event raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryBeaconUpgraded struct {
	Beacon common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterBeaconUpgraded is a free log retrieval operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) FilterBeaconUpgraded(opts *bind.FilterOpts, beacon []common.Address) (*LocalhostSpaceFactoryBeaconUpgradedIterator, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.FilterLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryBeaconUpgradedIterator{contract: _LocalhostSpaceFactory.contract, event: "BeaconUpgraded", logs: logs, sub: sub}, nil
}

// WatchBeaconUpgraded is a free log subscription operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) WatchBeaconUpgraded(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceFactoryBeaconUpgraded, beacon []common.Address) (event.Subscription, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.WatchLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceFactoryBeaconUpgraded)
				if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
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
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) ParseBeaconUpgraded(log types.Log) (*LocalhostSpaceFactoryBeaconUpgraded, error) {
	event := new(LocalhostSpaceFactoryBeaconUpgraded)
	if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceFactoryInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryInitializedIterator struct {
	Event *LocalhostSpaceFactoryInitialized // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceFactoryInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceFactoryInitialized)
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
		it.Event = new(LocalhostSpaceFactoryInitialized)
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
func (it *LocalhostSpaceFactoryInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceFactoryInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceFactoryInitialized represents a Initialized event raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryInitialized struct {
	Version uint8
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) FilterInitialized(opts *bind.FilterOpts) (*LocalhostSpaceFactoryInitializedIterator, error) {

	logs, sub, err := _LocalhostSpaceFactory.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryInitializedIterator{contract: _LocalhostSpaceFactory.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceFactoryInitialized) (event.Subscription, error) {

	logs, sub, err := _LocalhostSpaceFactory.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceFactoryInitialized)
				if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "Initialized", log); err != nil {
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
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) ParseInitialized(log types.Log) (*LocalhostSpaceFactoryInitialized, error) {
	event := new(LocalhostSpaceFactoryInitialized)
	if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceFactoryOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryOwnershipTransferredIterator struct {
	Event *LocalhostSpaceFactoryOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceFactoryOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceFactoryOwnershipTransferred)
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
		it.Event = new(LocalhostSpaceFactoryOwnershipTransferred)
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
func (it *LocalhostSpaceFactoryOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceFactoryOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceFactoryOwnershipTransferred represents a OwnershipTransferred event raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*LocalhostSpaceFactoryOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryOwnershipTransferredIterator{contract: _LocalhostSpaceFactory.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceFactoryOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceFactoryOwnershipTransferred)
				if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) ParseOwnershipTransferred(log types.Log) (*LocalhostSpaceFactoryOwnershipTransferred, error) {
	event := new(LocalhostSpaceFactoryOwnershipTransferred)
	if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceFactoryUpgradedIterator is returned from FilterUpgraded and is used to iterate over the raw logs and unpacked data for Upgraded events raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryUpgradedIterator struct {
	Event *LocalhostSpaceFactoryUpgraded // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceFactoryUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceFactoryUpgraded)
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
		it.Event = new(LocalhostSpaceFactoryUpgraded)
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
func (it *LocalhostSpaceFactoryUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceFactoryUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceFactoryUpgraded represents a Upgraded event raised by the LocalhostSpaceFactory contract.
type LocalhostSpaceFactoryUpgraded struct {
	Implementation common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgraded is a free log retrieval operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) FilterUpgraded(opts *bind.FilterOpts, implementation []common.Address) (*LocalhostSpaceFactoryUpgradedIterator, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.FilterLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFactoryUpgradedIterator{contract: _LocalhostSpaceFactory.contract, event: "Upgraded", logs: logs, sub: sub}, nil
}

// WatchUpgraded is a free log subscription operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) WatchUpgraded(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceFactoryUpgraded, implementation []common.Address) (event.Subscription, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _LocalhostSpaceFactory.contract.WatchLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceFactoryUpgraded)
				if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "Upgraded", log); err != nil {
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
func (_LocalhostSpaceFactory *LocalhostSpaceFactoryFilterer) ParseUpgraded(log types.Log) (*LocalhostSpaceFactoryUpgraded, error) {
	event := new(LocalhostSpaceFactoryUpgraded)
	if err := _LocalhostSpaceFactory.contract.UnpackLog(event, "Upgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
