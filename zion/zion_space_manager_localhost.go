// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package zion

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

// DataTypesCreateSpaceData is an auto generated low-level Go binding around an user-defined struct.
type DataTypesCreateSpaceData struct {
	SpaceName string
	NetworkId string
}

// DataTypesCreateSpaceTokenEntitlementData is an auto generated low-level Go binding around an user-defined struct.
type DataTypesCreateSpaceTokenEntitlementData struct {
	EntitlementModuleAddress common.Address
	TokenAddress             common.Address
	Quantity                 *big.Int
	Description              string
	EntitlementTypes         []uint8
}

// DataTypesEntitlementModuleInfo is an auto generated low-level Go binding around an user-defined struct.
type DataTypesEntitlementModuleInfo struct {
	EntitlementAddress     common.Address
	EntitlementName        string
	EntitlementDescription string
}

// DataTypesSpaceInfo is an auto generated low-level Go binding around an user-defined struct.
type DataTypesSpaceInfo struct {
	SpaceId   *big.Int
	CreatedAt *big.Int
	Name      string
	Creator   common.Address
	Owner     common.Address
}

// ZionSpaceManagerLocalhostMetaData contains all meta data concerning the ZionSpaceManagerLocalhost contract.
var ZionSpaceManagerLocalhostMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"DefaultEntitlementModuleNotSet\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementAlreadyWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementModuleNotSupported\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementNotWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotSpaceOwner\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"enumDataTypes.EntitlementType[]\",\"name\":\"entitlementTypes\",\"type\":\"uint8[]\"},{\"internalType\":\"bytes\",\"name\":\"entitlementData\",\"type\":\"bytes\"}],\"name\":\"addEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.CreateSpaceData\",\"name\":\"info\",\"type\":\"tuple\"}],\"name\":\"createSpace\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.CreateSpaceData\",\"name\":\"info\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"tokenAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"quantity\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"description\",\"type\":\"string\"},{\"internalType\":\"enumDataTypes.EntitlementType[]\",\"name\":\"entitlementTypes\",\"type\":\"uint8[]\"}],\"internalType\":\"structDataTypes.CreateSpaceTokenEntitlementData\",\"name\":\"entitlement\",\"type\":\"tuple\"}],\"name\":\"createSpaceWithTokenEntitlement\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"}],\"name\":\"getEntitlementsBySpaceId\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"entitlements\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"}],\"name\":\"getEntitlementsInfoBySpaceId\",\"outputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"entitlementAddress\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"entitlementName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"entitlementDescription\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.EntitlementModuleInfo[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"name\":\"getSpaceIdByNetworkId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_spaceId\",\"type\":\"uint256\"}],\"name\":\"getSpaceInfoBySpaceId\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"creator\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"internalType\":\"structDataTypes.SpaceInfo\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_spaceId\",\"type\":\"uint256\"}],\"name\":\"getSpaceOwnerBySpaceId\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"ownerAddress\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getSpaces\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"creator\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"internalType\":\"structDataTypes.SpaceInfo[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"roomId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"internalType\":\"enumDataTypes.EntitlementType\",\"name\":\"entitlementType\",\"type\":\"uint8\"}],\"name\":\"isEntitled\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"}],\"name\":\"isEntitlementModuleWhitelisted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"entitlementModule\",\"type\":\"address\"}],\"name\":\"registerDefaultEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"enumDataTypes.EntitlementType[]\",\"name\":\"entitlementTypes\",\"type\":\"uint8[]\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"removeEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementAddress\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"whitelist\",\"type\":\"bool\"}],\"name\":\"whitelistEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// ZionSpaceManagerLocalhostABI is the input ABI used to generate the binding from.
// Deprecated: Use ZionSpaceManagerLocalhostMetaData.ABI instead.
var ZionSpaceManagerLocalhostABI = ZionSpaceManagerLocalhostMetaData.ABI

// ZionSpaceManagerLocalhost is an auto generated Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhost struct {
	ZionSpaceManagerLocalhostCaller     // Read-only binding to the contract
	ZionSpaceManagerLocalhostTransactor // Write-only binding to the contract
	ZionSpaceManagerLocalhostFilterer   // Log filterer for contract events
}

// ZionSpaceManagerLocalhostCaller is an auto generated read-only Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhostCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerLocalhostTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhostTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerLocalhostFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ZionSpaceManagerLocalhostFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerLocalhostSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ZionSpaceManagerLocalhostSession struct {
	Contract     *ZionSpaceManagerLocalhost // Generic contract binding to set the session for
	CallOpts     bind.CallOpts              // Call options to use throughout this session
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// ZionSpaceManagerLocalhostCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ZionSpaceManagerLocalhostCallerSession struct {
	Contract *ZionSpaceManagerLocalhostCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                    // Call options to use throughout this session
}

// ZionSpaceManagerLocalhostTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ZionSpaceManagerLocalhostTransactorSession struct {
	Contract     *ZionSpaceManagerLocalhostTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                    // Transaction auth options to use throughout this session
}

// ZionSpaceManagerLocalhostRaw is an auto generated low-level Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhostRaw struct {
	Contract *ZionSpaceManagerLocalhost // Generic contract binding to access the raw methods on
}

// ZionSpaceManagerLocalhostCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhostCallerRaw struct {
	Contract *ZionSpaceManagerLocalhostCaller // Generic read-only contract binding to access the raw methods on
}

// ZionSpaceManagerLocalhostTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ZionSpaceManagerLocalhostTransactorRaw struct {
	Contract *ZionSpaceManagerLocalhostTransactor // Generic write-only contract binding to access the raw methods on
}

// NewZionSpaceManagerLocalhost creates a new instance of ZionSpaceManagerLocalhost, bound to a specific deployed contract.
func NewZionSpaceManagerLocalhost(address common.Address, backend bind.ContractBackend) (*ZionSpaceManagerLocalhost, error) {
	contract, err := bindZionSpaceManagerLocalhost(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerLocalhost{ZionSpaceManagerLocalhostCaller: ZionSpaceManagerLocalhostCaller{contract: contract}, ZionSpaceManagerLocalhostTransactor: ZionSpaceManagerLocalhostTransactor{contract: contract}, ZionSpaceManagerLocalhostFilterer: ZionSpaceManagerLocalhostFilterer{contract: contract}}, nil
}

// NewZionSpaceManagerLocalhostCaller creates a new read-only instance of ZionSpaceManagerLocalhost, bound to a specific deployed contract.
func NewZionSpaceManagerLocalhostCaller(address common.Address, caller bind.ContractCaller) (*ZionSpaceManagerLocalhostCaller, error) {
	contract, err := bindZionSpaceManagerLocalhost(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerLocalhostCaller{contract: contract}, nil
}

// NewZionSpaceManagerLocalhostTransactor creates a new write-only instance of ZionSpaceManagerLocalhost, bound to a specific deployed contract.
func NewZionSpaceManagerLocalhostTransactor(address common.Address, transactor bind.ContractTransactor) (*ZionSpaceManagerLocalhostTransactor, error) {
	contract, err := bindZionSpaceManagerLocalhost(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerLocalhostTransactor{contract: contract}, nil
}

// NewZionSpaceManagerLocalhostFilterer creates a new log filterer instance of ZionSpaceManagerLocalhost, bound to a specific deployed contract.
func NewZionSpaceManagerLocalhostFilterer(address common.Address, filterer bind.ContractFilterer) (*ZionSpaceManagerLocalhostFilterer, error) {
	contract, err := bindZionSpaceManagerLocalhost(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerLocalhostFilterer{contract: contract}, nil
}

// bindZionSpaceManagerLocalhost binds a generic wrapper to an already deployed contract.
func bindZionSpaceManagerLocalhost(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ZionSpaceManagerLocalhostABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ZionSpaceManagerLocalhost.Contract.ZionSpaceManagerLocalhostCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.ZionSpaceManagerLocalhostTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.ZionSpaceManagerLocalhostTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ZionSpaceManagerLocalhost.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.contract.Transact(opts, method, params...)
}

// GetEntitlementsBySpaceId is a free data retrieval call binding the contract method 0x35205b43.
//
// Solidity: function getEntitlementsBySpaceId(uint256 spaceId) view returns(address[] entitlements)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetEntitlementsBySpaceId(opts *bind.CallOpts, spaceId *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getEntitlementsBySpaceId", spaceId)

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetEntitlementsBySpaceId is a free data retrieval call binding the contract method 0x35205b43.
//
// Solidity: function getEntitlementsBySpaceId(uint256 spaceId) view returns(address[] entitlements)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetEntitlementsBySpaceId(spaceId *big.Int) ([]common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetEntitlementsBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, spaceId)
}

// GetEntitlementsBySpaceId is a free data retrieval call binding the contract method 0x35205b43.
//
// Solidity: function getEntitlementsBySpaceId(uint256 spaceId) view returns(address[] entitlements)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetEntitlementsBySpaceId(spaceId *big.Int) ([]common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetEntitlementsBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, spaceId)
}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetEntitlementsInfoBySpaceId(opts *bind.CallOpts, spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getEntitlementsInfoBySpaceId", spaceId)

	if err != nil {
		return *new([]DataTypesEntitlementModuleInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesEntitlementModuleInfo)).(*[]DataTypesEntitlementModuleInfo)

	return out0, err

}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetEntitlementsInfoBySpaceId(spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetEntitlementsInfoBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, spaceId)
}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetEntitlementsInfoBySpaceId(spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetEntitlementsInfoBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, spaceId)
}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetSpaceIdByNetworkId(opts *bind.CallOpts, networkId string) (*big.Int, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getSpaceIdByNetworkId", networkId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetSpaceIdByNetworkId(networkId string) (*big.Int, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceIdByNetworkId(&_ZionSpaceManagerLocalhost.CallOpts, networkId)
}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetSpaceIdByNetworkId(networkId string) (*big.Int, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceIdByNetworkId(&_ZionSpaceManagerLocalhost.CallOpts, networkId)
}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetSpaceInfoBySpaceId(opts *bind.CallOpts, _spaceId *big.Int) (DataTypesSpaceInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getSpaceInfoBySpaceId", _spaceId)

	if err != nil {
		return *new(DataTypesSpaceInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesSpaceInfo)).(*DataTypesSpaceInfo)

	return out0, err

}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetSpaceInfoBySpaceId(_spaceId *big.Int) (DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceInfoBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, _spaceId)
}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetSpaceInfoBySpaceId(_spaceId *big.Int) (DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceInfoBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, _spaceId)
}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetSpaceOwnerBySpaceId(opts *bind.CallOpts, _spaceId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getSpaceOwnerBySpaceId", _spaceId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetSpaceOwnerBySpaceId(_spaceId *big.Int) (common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceOwnerBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, _spaceId)
}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetSpaceOwnerBySpaceId(_spaceId *big.Int) (common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaceOwnerBySpaceId(&_ZionSpaceManagerLocalhost.CallOpts, _spaceId)
}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) GetSpaces(opts *bind.CallOpts) ([]DataTypesSpaceInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "getSpaces")

	if err != nil {
		return *new([]DataTypesSpaceInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesSpaceInfo)).(*[]DataTypesSpaceInfo)

	return out0, err

}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) GetSpaces() ([]DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaces(&_ZionSpaceManagerLocalhost.CallOpts)
}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) GetSpaces() ([]DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerLocalhost.Contract.GetSpaces(&_ZionSpaceManagerLocalhost.CallOpts)
}

// IsEntitled is a free data retrieval call binding the contract method 0x6f09c765.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, uint8 entitlementType) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) IsEntitled(opts *bind.CallOpts, spaceId *big.Int, roomId *big.Int, user common.Address, entitlementType uint8) (bool, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "isEntitled", spaceId, roomId, user, entitlementType)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitled is a free data retrieval call binding the contract method 0x6f09c765.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, uint8 entitlementType) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) IsEntitled(spaceId *big.Int, roomId *big.Int, user common.Address, entitlementType uint8) (bool, error) {
	return _ZionSpaceManagerLocalhost.Contract.IsEntitled(&_ZionSpaceManagerLocalhost.CallOpts, spaceId, roomId, user, entitlementType)
}

// IsEntitled is a free data retrieval call binding the contract method 0x6f09c765.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, uint8 entitlementType) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) IsEntitled(spaceId *big.Int, roomId *big.Int, user common.Address, entitlementType uint8) (bool, error) {
	return _ZionSpaceManagerLocalhost.Contract.IsEntitled(&_ZionSpaceManagerLocalhost.CallOpts, spaceId, roomId, user, entitlementType)
}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) IsEntitlementModuleWhitelisted(opts *bind.CallOpts, spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "isEntitlementModuleWhitelisted", spaceId, entitlementModuleAddress)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) IsEntitlementModuleWhitelisted(spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	return _ZionSpaceManagerLocalhost.Contract.IsEntitlementModuleWhitelisted(&_ZionSpaceManagerLocalhost.CallOpts, spaceId, entitlementModuleAddress)
}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) IsEntitlementModuleWhitelisted(spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	return _ZionSpaceManagerLocalhost.Contract.IsEntitlementModuleWhitelisted(&_ZionSpaceManagerLocalhost.CallOpts, spaceId, entitlementModuleAddress)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerLocalhost.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) Owner() (common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.Owner(&_ZionSpaceManagerLocalhost.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostCallerSession) Owner() (common.Address, error) {
	return _ZionSpaceManagerLocalhost.Contract.Owner(&_ZionSpaceManagerLocalhost.CallOpts)
}

// AddEntitlement is a paid mutator transaction binding the contract method 0xa382f501.
//
// Solidity: function addEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes entitlementData) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) AddEntitlement(opts *bind.TransactOpts, spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "addEntitlement", spaceId, entitlementModuleAddress, entitlementTypes, entitlementData)
}

// AddEntitlement is a paid mutator transaction binding the contract method 0xa382f501.
//
// Solidity: function addEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes entitlementData) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) AddEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.AddEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementModuleAddress, entitlementTypes, entitlementData)
}

// AddEntitlement is a paid mutator transaction binding the contract method 0xa382f501.
//
// Solidity: function addEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes entitlementData) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) AddEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.AddEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementModuleAddress, entitlementTypes, entitlementData)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) CreateSpace(opts *bind.TransactOpts, info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "createSpace", info)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) CreateSpace(info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.CreateSpace(&_ZionSpaceManagerLocalhost.TransactOpts, info)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) CreateSpace(info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.CreateSpace(&_ZionSpaceManagerLocalhost.TransactOpts, info)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x11c20f79.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,uint8[]) entitlement) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) CreateSpaceWithTokenEntitlement(opts *bind.TransactOpts, info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "createSpaceWithTokenEntitlement", info, entitlement)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x11c20f79.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,uint8[]) entitlement) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) CreateSpaceWithTokenEntitlement(info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.CreateSpaceWithTokenEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, info, entitlement)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x11c20f79.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,uint8[]) entitlement) returns(uint256)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) CreateSpaceWithTokenEntitlement(info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.CreateSpaceWithTokenEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, info, entitlement)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) RegisterDefaultEntitlementModule(opts *bind.TransactOpts, entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "registerDefaultEntitlementModule", entitlementModule)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) RegisterDefaultEntitlementModule(entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RegisterDefaultEntitlementModule(&_ZionSpaceManagerLocalhost.TransactOpts, entitlementModule)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) RegisterDefaultEntitlementModule(entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RegisterDefaultEntitlementModule(&_ZionSpaceManagerLocalhost.TransactOpts, entitlementModule)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0xcd4deca6.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes data) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) RemoveEntitlement(opts *bind.TransactOpts, spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "removeEntitlement", spaceId, entitlementModuleAddress, entitlementTypes, data)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0xcd4deca6.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes data) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) RemoveEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RemoveEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementModuleAddress, entitlementTypes, data)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0xcd4deca6.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint8[] entitlementTypes, bytes data) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) RemoveEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, entitlementTypes []uint8, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RemoveEntitlement(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementModuleAddress, entitlementTypes, data)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) RenounceOwnership() (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RenounceOwnership(&_ZionSpaceManagerLocalhost.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.RenounceOwnership(&_ZionSpaceManagerLocalhost.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.TransferOwnership(&_ZionSpaceManagerLocalhost.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.TransferOwnership(&_ZionSpaceManagerLocalhost.TransactOpts, newOwner)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactor) WhitelistEntitlementModule(opts *bind.TransactOpts, spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.contract.Transact(opts, "whitelistEntitlementModule", spaceId, entitlementAddress, whitelist)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostSession) WhitelistEntitlementModule(spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.WhitelistEntitlementModule(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementAddress, whitelist)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostTransactorSession) WhitelistEntitlementModule(spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerLocalhost.Contract.WhitelistEntitlementModule(&_ZionSpaceManagerLocalhost.TransactOpts, spaceId, entitlementAddress, whitelist)
}

// ZionSpaceManagerLocalhostOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the ZionSpaceManagerLocalhost contract.
type ZionSpaceManagerLocalhostOwnershipTransferredIterator struct {
	Event *ZionSpaceManagerLocalhostOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *ZionSpaceManagerLocalhostOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ZionSpaceManagerLocalhostOwnershipTransferred)
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
		it.Event = new(ZionSpaceManagerLocalhostOwnershipTransferred)
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
func (it *ZionSpaceManagerLocalhostOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ZionSpaceManagerLocalhostOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ZionSpaceManagerLocalhostOwnershipTransferred represents a OwnershipTransferred event raised by the ZionSpaceManagerLocalhost contract.
type ZionSpaceManagerLocalhostOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*ZionSpaceManagerLocalhostOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ZionSpaceManagerLocalhost.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerLocalhostOwnershipTransferredIterator{contract: _ZionSpaceManagerLocalhost.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *ZionSpaceManagerLocalhostOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ZionSpaceManagerLocalhost.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ZionSpaceManagerLocalhostOwnershipTransferred)
				if err := _ZionSpaceManagerLocalhost.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_ZionSpaceManagerLocalhost *ZionSpaceManagerLocalhostFilterer) ParseOwnershipTransferred(log types.Log) (*ZionSpaceManagerLocalhostOwnershipTransferred, error) {
	event := new(ZionSpaceManagerLocalhostOwnershipTransferred)
	if err := _ZionSpaceManagerLocalhost.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
