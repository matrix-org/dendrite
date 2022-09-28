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


// DataTypesPermission is an auto generated low-level Go binding around an user-defined struct.
type DataTypesPermission struct {
	Name string
}

// DataTypesRole is an auto generated low-level Go binding around an user-defined struct.
type DataTypesRole struct {
	RoleId       *big.Int
	Name         string
	Color        [8]byte
	IsTransitive bool
}

// ZionSpaceManagerGoerliMetaData contains all meta data concerning the ZionSpaceManagerGoerli contract.
var ZionSpaceManagerGoerliMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"DefaultEntitlementModuleNotSet\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementAlreadyWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementModuleNotSupported\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementNotWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotSpaceOwner\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Permission\",\"name\":\"permission\",\"type\":\"tuple\"}],\"name\":\"addPermissionToRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"entitlementData\",\"type\":\"bytes\"}],\"name\":\"addRoleToEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"bytes8\",\"name\":\"color\",\"type\":\"bytes8\"}],\"name\":\"createRole\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.CreateSpaceData\",\"name\":\"info\",\"type\":\"tuple\"}],\"name\":\"createSpace\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"spaceName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.CreateSpaceData\",\"name\":\"info\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"tokenAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"quantity\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"description\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"permissions\",\"type\":\"string[]\"}],\"internalType\":\"structDataTypes.CreateSpaceTokenEntitlementData\",\"name\":\"entitlement\",\"type\":\"tuple\"}],\"name\":\"createSpaceWithTokenEntitlement\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"}],\"name\":\"getEntitlementModulesBySpaceId\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"entitlementModules\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"}],\"name\":\"getEntitlementsInfoBySpaceId\",\"outputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"entitlementAddress\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"entitlementName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"entitlementDescription\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.EntitlementModuleInfo[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumISpaceManager.ZionPermission\",\"name\":\"zionPermission\",\"type\":\"uint8\"}],\"name\":\"getPermissionFromMap\",\"outputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Permission\",\"name\":\"permission\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"}],\"name\":\"getPermissionsBySpaceIdByRoleId\",\"outputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Permission[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"}],\"name\":\"getRoleBySpaceIdByRoleId\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"bytes8\",\"name\":\"color\",\"type\":\"bytes8\"},{\"internalType\":\"bool\",\"name\":\"isTransitive\",\"type\":\"bool\"}],\"internalType\":\"structDataTypes.Role\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"}],\"name\":\"getRolesBySpaceId\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"bytes8\",\"name\":\"color\",\"type\":\"bytes8\"},{\"internalType\":\"bool\",\"name\":\"isTransitive\",\"type\":\"bool\"}],\"internalType\":\"structDataTypes.Role[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"networkId\",\"type\":\"string\"}],\"name\":\"getSpaceIdByNetworkId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_spaceId\",\"type\":\"uint256\"}],\"name\":\"getSpaceInfoBySpaceId\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"creator\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"internalType\":\"structDataTypes.SpaceInfo\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_spaceId\",\"type\":\"uint256\"}],\"name\":\"getSpaceOwnerBySpaceId\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"ownerAddress\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getSpaces\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"creator\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"internalType\":\"structDataTypes.SpaceInfo[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"roomId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"user\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Permission\",\"name\":\"permission\",\"type\":\"tuple\"}],\"name\":\"isEntitled\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"}],\"name\":\"isEntitlementModuleWhitelisted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"entitlementModule\",\"type\":\"address\"}],\"name\":\"registerDefaultEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementModuleAddress\",\"type\":\"address\"},{\"internalType\":\"uint256[]\",\"name\":\"roleIds\",\"type\":\"uint256[]\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"removeEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"spaceId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"entitlementAddress\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"whitelist\",\"type\":\"bool\"}],\"name\":\"whitelistEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumISpaceManager.ZionPermission\",\"name\":\"\",\"type\":\"uint8\"}],\"name\":\"zionPermissionsMap\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// ZionSpaceManagerGoerliABI is the input ABI used to generate the binding from.
// Deprecated: Use ZionSpaceManagerGoerliMetaData.ABI instead.
var ZionSpaceManagerGoerliABI = ZionSpaceManagerGoerliMetaData.ABI

// ZionSpaceManagerGoerli is an auto generated Go binding around an Ethereum contract.
type ZionSpaceManagerGoerli struct {
	ZionSpaceManagerGoerliCaller     // Read-only binding to the contract
	ZionSpaceManagerGoerliTransactor // Write-only binding to the contract
	ZionSpaceManagerGoerliFilterer   // Log filterer for contract events
}

// ZionSpaceManagerGoerliCaller is an auto generated read-only Go binding around an Ethereum contract.
type ZionSpaceManagerGoerliCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerGoerliTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ZionSpaceManagerGoerliTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerGoerliFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ZionSpaceManagerGoerliFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ZionSpaceManagerGoerliSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ZionSpaceManagerGoerliSession struct {
	Contract     *ZionSpaceManagerGoerli // Generic contract binding to set the session for
	CallOpts     bind.CallOpts           // Call options to use throughout this session
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// ZionSpaceManagerGoerliCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ZionSpaceManagerGoerliCallerSession struct {
	Contract *ZionSpaceManagerGoerliCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                 // Call options to use throughout this session
}

// ZionSpaceManagerGoerliTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ZionSpaceManagerGoerliTransactorSession struct {
	Contract     *ZionSpaceManagerGoerliTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                 // Transaction auth options to use throughout this session
}

// ZionSpaceManagerGoerliRaw is an auto generated low-level Go binding around an Ethereum contract.
type ZionSpaceManagerGoerliRaw struct {
	Contract *ZionSpaceManagerGoerli // Generic contract binding to access the raw methods on
}

// ZionSpaceManagerGoerliCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ZionSpaceManagerGoerliCallerRaw struct {
	Contract *ZionSpaceManagerGoerliCaller // Generic read-only contract binding to access the raw methods on
}

// ZionSpaceManagerGoerliTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ZionSpaceManagerGoerliTransactorRaw struct {
	Contract *ZionSpaceManagerGoerliTransactor // Generic write-only contract binding to access the raw methods on
}

// NewZionSpaceManagerGoerli creates a new instance of ZionSpaceManagerGoerli, bound to a specific deployed contract.
func NewZionSpaceManagerGoerli(address common.Address, backend bind.ContractBackend) (*ZionSpaceManagerGoerli, error) {
	contract, err := bindZionSpaceManagerGoerli(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerGoerli{ZionSpaceManagerGoerliCaller: ZionSpaceManagerGoerliCaller{contract: contract}, ZionSpaceManagerGoerliTransactor: ZionSpaceManagerGoerliTransactor{contract: contract}, ZionSpaceManagerGoerliFilterer: ZionSpaceManagerGoerliFilterer{contract: contract}}, nil
}

// NewZionSpaceManagerGoerliCaller creates a new read-only instance of ZionSpaceManagerGoerli, bound to a specific deployed contract.
func NewZionSpaceManagerGoerliCaller(address common.Address, caller bind.ContractCaller) (*ZionSpaceManagerGoerliCaller, error) {
	contract, err := bindZionSpaceManagerGoerli(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerGoerliCaller{contract: contract}, nil
}

// NewZionSpaceManagerGoerliTransactor creates a new write-only instance of ZionSpaceManagerGoerli, bound to a specific deployed contract.
func NewZionSpaceManagerGoerliTransactor(address common.Address, transactor bind.ContractTransactor) (*ZionSpaceManagerGoerliTransactor, error) {
	contract, err := bindZionSpaceManagerGoerli(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerGoerliTransactor{contract: contract}, nil
}

// NewZionSpaceManagerGoerliFilterer creates a new log filterer instance of ZionSpaceManagerGoerli, bound to a specific deployed contract.
func NewZionSpaceManagerGoerliFilterer(address common.Address, filterer bind.ContractFilterer) (*ZionSpaceManagerGoerliFilterer, error) {
	contract, err := bindZionSpaceManagerGoerli(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerGoerliFilterer{contract: contract}, nil
}

// bindZionSpaceManagerGoerli binds a generic wrapper to an already deployed contract.
func bindZionSpaceManagerGoerli(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ZionSpaceManagerGoerliABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ZionSpaceManagerGoerli.Contract.ZionSpaceManagerGoerliCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.ZionSpaceManagerGoerliTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.ZionSpaceManagerGoerliTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ZionSpaceManagerGoerli.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.contract.Transact(opts, method, params...)
}

// GetEntitlementModulesBySpaceId is a free data retrieval call binding the contract method 0x41ab8a9a.
//
// Solidity: function getEntitlementModulesBySpaceId(uint256 spaceId) view returns(address[] entitlementModules)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetEntitlementModulesBySpaceId(opts *bind.CallOpts, spaceId *big.Int) ([]common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getEntitlementModulesBySpaceId", spaceId)

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetEntitlementModulesBySpaceId is a free data retrieval call binding the contract method 0x41ab8a9a.
//
// Solidity: function getEntitlementModulesBySpaceId(uint256 spaceId) view returns(address[] entitlementModules)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetEntitlementModulesBySpaceId(spaceId *big.Int) ([]common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.GetEntitlementModulesBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetEntitlementModulesBySpaceId is a free data retrieval call binding the contract method 0x41ab8a9a.
//
// Solidity: function getEntitlementModulesBySpaceId(uint256 spaceId) view returns(address[] entitlementModules)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetEntitlementModulesBySpaceId(spaceId *big.Int) ([]common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.GetEntitlementModulesBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetEntitlementsInfoBySpaceId(opts *bind.CallOpts, spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getEntitlementsInfoBySpaceId", spaceId)

	if err != nil {
		return *new([]DataTypesEntitlementModuleInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesEntitlementModuleInfo)).(*[]DataTypesEntitlementModuleInfo)

	return out0, err

}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetEntitlementsInfoBySpaceId(spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetEntitlementsInfoBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetEntitlementsInfoBySpaceId is a free data retrieval call binding the contract method 0x8a99bd88.
//
// Solidity: function getEntitlementsInfoBySpaceId(uint256 spaceId) view returns((address,string,string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetEntitlementsInfoBySpaceId(spaceId *big.Int) ([]DataTypesEntitlementModuleInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetEntitlementsInfoBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetPermissionFromMap is a free data retrieval call binding the contract method 0x9152efda.
//
// Solidity: function getPermissionFromMap(uint8 zionPermission) view returns((string) permission)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetPermissionFromMap(opts *bind.CallOpts, zionPermission uint8) (DataTypesPermission, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getPermissionFromMap", zionPermission)

	if err != nil {
		return *new(DataTypesPermission), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesPermission)).(*DataTypesPermission)

	return out0, err

}

// GetPermissionFromMap is a free data retrieval call binding the contract method 0x9152efda.
//
// Solidity: function getPermissionFromMap(uint8 zionPermission) view returns((string) permission)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetPermissionFromMap(zionPermission uint8) (DataTypesPermission, error) {
	return _ZionSpaceManagerGoerli.Contract.GetPermissionFromMap(&_ZionSpaceManagerGoerli.CallOpts, zionPermission)
}

// GetPermissionFromMap is a free data retrieval call binding the contract method 0x9152efda.
//
// Solidity: function getPermissionFromMap(uint8 zionPermission) view returns((string) permission)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetPermissionFromMap(zionPermission uint8) (DataTypesPermission, error) {
	return _ZionSpaceManagerGoerli.Contract.GetPermissionFromMap(&_ZionSpaceManagerGoerli.CallOpts, zionPermission)
}

// GetPermissionsBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xa7d325fc.
//
// Solidity: function getPermissionsBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetPermissionsBySpaceIdByRoleId(opts *bind.CallOpts, spaceId *big.Int, roleId *big.Int) ([]DataTypesPermission, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getPermissionsBySpaceIdByRoleId", spaceId, roleId)

	if err != nil {
		return *new([]DataTypesPermission), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesPermission)).(*[]DataTypesPermission)

	return out0, err

}

// GetPermissionsBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xa7d325fc.
//
// Solidity: function getPermissionsBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetPermissionsBySpaceIdByRoleId(spaceId *big.Int, roleId *big.Int) ([]DataTypesPermission, error) {
	return _ZionSpaceManagerGoerli.Contract.GetPermissionsBySpaceIdByRoleId(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roleId)
}

// GetPermissionsBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xa7d325fc.
//
// Solidity: function getPermissionsBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((string)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetPermissionsBySpaceIdByRoleId(spaceId *big.Int, roleId *big.Int) ([]DataTypesPermission, error) {
	return _ZionSpaceManagerGoerli.Contract.GetPermissionsBySpaceIdByRoleId(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roleId)
}

// GetRoleBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xd9af4ecf.
//
// Solidity: function getRoleBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((uint256,string,bytes8,bool))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetRoleBySpaceIdByRoleId(opts *bind.CallOpts, spaceId *big.Int, roleId *big.Int) (DataTypesRole, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getRoleBySpaceIdByRoleId", spaceId, roleId)

	if err != nil {
		return *new(DataTypesRole), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesRole)).(*DataTypesRole)

	return out0, err

}

// GetRoleBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xd9af4ecf.
//
// Solidity: function getRoleBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((uint256,string,bytes8,bool))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetRoleBySpaceIdByRoleId(spaceId *big.Int, roleId *big.Int) (DataTypesRole, error) {
	return _ZionSpaceManagerGoerli.Contract.GetRoleBySpaceIdByRoleId(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roleId)
}

// GetRoleBySpaceIdByRoleId is a free data retrieval call binding the contract method 0xd9af4ecf.
//
// Solidity: function getRoleBySpaceIdByRoleId(uint256 spaceId, uint256 roleId) view returns((uint256,string,bytes8,bool))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetRoleBySpaceIdByRoleId(spaceId *big.Int, roleId *big.Int) (DataTypesRole, error) {
	return _ZionSpaceManagerGoerli.Contract.GetRoleBySpaceIdByRoleId(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roleId)
}

// GetRolesBySpaceId is a free data retrieval call binding the contract method 0x52f1bea2.
//
// Solidity: function getRolesBySpaceId(uint256 spaceId) view returns((uint256,string,bytes8,bool)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetRolesBySpaceId(opts *bind.CallOpts, spaceId *big.Int) ([]DataTypesRole, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getRolesBySpaceId", spaceId)

	if err != nil {
		return *new([]DataTypesRole), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesRole)).(*[]DataTypesRole)

	return out0, err

}

// GetRolesBySpaceId is a free data retrieval call binding the contract method 0x52f1bea2.
//
// Solidity: function getRolesBySpaceId(uint256 spaceId) view returns((uint256,string,bytes8,bool)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetRolesBySpaceId(spaceId *big.Int) ([]DataTypesRole, error) {
	return _ZionSpaceManagerGoerli.Contract.GetRolesBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetRolesBySpaceId is a free data retrieval call binding the contract method 0x52f1bea2.
//
// Solidity: function getRolesBySpaceId(uint256 spaceId) view returns((uint256,string,bytes8,bool)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetRolesBySpaceId(spaceId *big.Int) ([]DataTypesRole, error) {
	return _ZionSpaceManagerGoerli.Contract.GetRolesBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, spaceId)
}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetSpaceIdByNetworkId(opts *bind.CallOpts, networkId string) (*big.Int, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getSpaceIdByNetworkId", networkId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetSpaceIdByNetworkId(networkId string) (*big.Int, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceIdByNetworkId(&_ZionSpaceManagerGoerli.CallOpts, networkId)
}

// GetSpaceIdByNetworkId is a free data retrieval call binding the contract method 0x9ddd0d6b.
//
// Solidity: function getSpaceIdByNetworkId(string networkId) view returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetSpaceIdByNetworkId(networkId string) (*big.Int, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceIdByNetworkId(&_ZionSpaceManagerGoerli.CallOpts, networkId)
}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetSpaceInfoBySpaceId(opts *bind.CallOpts, _spaceId *big.Int) (DataTypesSpaceInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getSpaceInfoBySpaceId", _spaceId)

	if err != nil {
		return *new(DataTypesSpaceInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesSpaceInfo)).(*DataTypesSpaceInfo)

	return out0, err

}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetSpaceInfoBySpaceId(_spaceId *big.Int) (DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceInfoBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, _spaceId)
}

// GetSpaceInfoBySpaceId is a free data retrieval call binding the contract method 0x3439f03b.
//
// Solidity: function getSpaceInfoBySpaceId(uint256 _spaceId) view returns((uint256,uint256,string,address,address))
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetSpaceInfoBySpaceId(_spaceId *big.Int) (DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceInfoBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, _spaceId)
}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetSpaceOwnerBySpaceId(opts *bind.CallOpts, _spaceId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getSpaceOwnerBySpaceId", _spaceId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetSpaceOwnerBySpaceId(_spaceId *big.Int) (common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceOwnerBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, _spaceId)
}

// GetSpaceOwnerBySpaceId is a free data retrieval call binding the contract method 0x7dde72d8.
//
// Solidity: function getSpaceOwnerBySpaceId(uint256 _spaceId) view returns(address ownerAddress)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetSpaceOwnerBySpaceId(_spaceId *big.Int) (common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaceOwnerBySpaceId(&_ZionSpaceManagerGoerli.CallOpts, _spaceId)
}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) GetSpaces(opts *bind.CallOpts) ([]DataTypesSpaceInfo, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "getSpaces")

	if err != nil {
		return *new([]DataTypesSpaceInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesSpaceInfo)).(*[]DataTypesSpaceInfo)

	return out0, err

}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) GetSpaces() ([]DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaces(&_ZionSpaceManagerGoerli.CallOpts)
}

// GetSpaces is a free data retrieval call binding the contract method 0x15478ca9.
//
// Solidity: function getSpaces() view returns((uint256,uint256,string,address,address)[])
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) GetSpaces() ([]DataTypesSpaceInfo, error) {
	return _ZionSpaceManagerGoerli.Contract.GetSpaces(&_ZionSpaceManagerGoerli.CallOpts)
}

// IsEntitled is a free data retrieval call binding the contract method 0x66218999.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, (string) permission) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) IsEntitled(opts *bind.CallOpts, spaceId *big.Int, roomId *big.Int, user common.Address, permission DataTypesPermission) (bool, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "isEntitled", spaceId, roomId, user, permission)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitled is a free data retrieval call binding the contract method 0x66218999.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, (string) permission) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) IsEntitled(spaceId *big.Int, roomId *big.Int, user common.Address, permission DataTypesPermission) (bool, error) {
	return _ZionSpaceManagerGoerli.Contract.IsEntitled(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roomId, user, permission)
}

// IsEntitled is a free data retrieval call binding the contract method 0x66218999.
//
// Solidity: function isEntitled(uint256 spaceId, uint256 roomId, address user, (string) permission) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) IsEntitled(spaceId *big.Int, roomId *big.Int, user common.Address, permission DataTypesPermission) (bool, error) {
	return _ZionSpaceManagerGoerli.Contract.IsEntitled(&_ZionSpaceManagerGoerli.CallOpts, spaceId, roomId, user, permission)
}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) IsEntitlementModuleWhitelisted(opts *bind.CallOpts, spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "isEntitlementModuleWhitelisted", spaceId, entitlementModuleAddress)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) IsEntitlementModuleWhitelisted(spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	return _ZionSpaceManagerGoerli.Contract.IsEntitlementModuleWhitelisted(&_ZionSpaceManagerGoerli.CallOpts, spaceId, entitlementModuleAddress)
}

// IsEntitlementModuleWhitelisted is a free data retrieval call binding the contract method 0xb010ac47.
//
// Solidity: function isEntitlementModuleWhitelisted(uint256 spaceId, address entitlementModuleAddress) view returns(bool)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) IsEntitlementModuleWhitelisted(spaceId *big.Int, entitlementModuleAddress common.Address) (bool, error) {
	return _ZionSpaceManagerGoerli.Contract.IsEntitlementModuleWhitelisted(&_ZionSpaceManagerGoerli.CallOpts, spaceId, entitlementModuleAddress)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) Owner() (common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.Owner(&_ZionSpaceManagerGoerli.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) Owner() (common.Address, error) {
	return _ZionSpaceManagerGoerli.Contract.Owner(&_ZionSpaceManagerGoerli.CallOpts)
}

// ZionPermissionsMap is a free data retrieval call binding the contract method 0xc870e04c.
//
// Solidity: function zionPermissionsMap(uint8 ) view returns(string name)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCaller) ZionPermissionsMap(opts *bind.CallOpts, arg0 uint8) (string, error) {
	var out []interface{}
	err := _ZionSpaceManagerGoerli.contract.Call(opts, &out, "zionPermissionsMap", arg0)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// ZionPermissionsMap is a free data retrieval call binding the contract method 0xc870e04c.
//
// Solidity: function zionPermissionsMap(uint8 ) view returns(string name)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) ZionPermissionsMap(arg0 uint8) (string, error) {
	return _ZionSpaceManagerGoerli.Contract.ZionPermissionsMap(&_ZionSpaceManagerGoerli.CallOpts, arg0)
}

// ZionPermissionsMap is a free data retrieval call binding the contract method 0xc870e04c.
//
// Solidity: function zionPermissionsMap(uint8 ) view returns(string name)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliCallerSession) ZionPermissionsMap(arg0 uint8) (string, error) {
	return _ZionSpaceManagerGoerli.Contract.ZionPermissionsMap(&_ZionSpaceManagerGoerli.CallOpts, arg0)
}

// AddPermissionToRole is a paid mutator transaction binding the contract method 0x3368c918.
//
// Solidity: function addPermissionToRole(uint256 spaceId, uint256 roleId, (string) permission) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) AddPermissionToRole(opts *bind.TransactOpts, spaceId *big.Int, roleId *big.Int, permission DataTypesPermission) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "addPermissionToRole", spaceId, roleId, permission)
}

// AddPermissionToRole is a paid mutator transaction binding the contract method 0x3368c918.
//
// Solidity: function addPermissionToRole(uint256 spaceId, uint256 roleId, (string) permission) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) AddPermissionToRole(spaceId *big.Int, roleId *big.Int, permission DataTypesPermission) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.AddPermissionToRole(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, roleId, permission)
}

// AddPermissionToRole is a paid mutator transaction binding the contract method 0x3368c918.
//
// Solidity: function addPermissionToRole(uint256 spaceId, uint256 roleId, (string) permission) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) AddPermissionToRole(spaceId *big.Int, roleId *big.Int, permission DataTypesPermission) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.AddPermissionToRole(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, roleId, permission)
}

// AddRoleToEntitlementModule is a paid mutator transaction binding the contract method 0xa7972473.
//
// Solidity: function addRoleToEntitlementModule(uint256 spaceId, address entitlementModuleAddress, uint256 roleId, bytes entitlementData) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) AddRoleToEntitlementModule(opts *bind.TransactOpts, spaceId *big.Int, entitlementModuleAddress common.Address, roleId *big.Int, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "addRoleToEntitlementModule", spaceId, entitlementModuleAddress, roleId, entitlementData)
}

// AddRoleToEntitlementModule is a paid mutator transaction binding the contract method 0xa7972473.
//
// Solidity: function addRoleToEntitlementModule(uint256 spaceId, address entitlementModuleAddress, uint256 roleId, bytes entitlementData) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) AddRoleToEntitlementModule(spaceId *big.Int, entitlementModuleAddress common.Address, roleId *big.Int, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.AddRoleToEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementModuleAddress, roleId, entitlementData)
}

// AddRoleToEntitlementModule is a paid mutator transaction binding the contract method 0xa7972473.
//
// Solidity: function addRoleToEntitlementModule(uint256 spaceId, address entitlementModuleAddress, uint256 roleId, bytes entitlementData) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) AddRoleToEntitlementModule(spaceId *big.Int, entitlementModuleAddress common.Address, roleId *big.Int, entitlementData []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.AddRoleToEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementModuleAddress, roleId, entitlementData)
}

// CreateRole is a paid mutator transaction binding the contract method 0xa3f13435.
//
// Solidity: function createRole(uint256 spaceId, string name, bytes8 color) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) CreateRole(opts *bind.TransactOpts, spaceId *big.Int, name string, color [8]byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "createRole", spaceId, name, color)
}

// CreateRole is a paid mutator transaction binding the contract method 0xa3f13435.
//
// Solidity: function createRole(uint256 spaceId, string name, bytes8 color) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) CreateRole(spaceId *big.Int, name string, color [8]byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateRole(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, name, color)
}

// CreateRole is a paid mutator transaction binding the contract method 0xa3f13435.
//
// Solidity: function createRole(uint256 spaceId, string name, bytes8 color) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) CreateRole(spaceId *big.Int, name string, color [8]byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateRole(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, name, color)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) CreateSpace(opts *bind.TransactOpts, info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "createSpace", info)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) CreateSpace(info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateSpace(&_ZionSpaceManagerGoerli.TransactOpts, info)
}

// CreateSpace is a paid mutator transaction binding the contract method 0x50b88cf7.
//
// Solidity: function createSpace((string,string) info) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) CreateSpace(info DataTypesCreateSpaceData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateSpace(&_ZionSpaceManagerGoerli.TransactOpts, info)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x7e9ea5c7.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,string[]) entitlement) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) CreateSpaceWithTokenEntitlement(opts *bind.TransactOpts, info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "createSpaceWithTokenEntitlement", info, entitlement)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x7e9ea5c7.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,string[]) entitlement) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) CreateSpaceWithTokenEntitlement(info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateSpaceWithTokenEntitlement(&_ZionSpaceManagerGoerli.TransactOpts, info, entitlement)
}

// CreateSpaceWithTokenEntitlement is a paid mutator transaction binding the contract method 0x7e9ea5c7.
//
// Solidity: function createSpaceWithTokenEntitlement((string,string) info, (address,address,uint256,string,string[]) entitlement) returns(uint256)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) CreateSpaceWithTokenEntitlement(info DataTypesCreateSpaceData, entitlement DataTypesCreateSpaceTokenEntitlementData) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.CreateSpaceWithTokenEntitlement(&_ZionSpaceManagerGoerli.TransactOpts, info, entitlement)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) RegisterDefaultEntitlementModule(opts *bind.TransactOpts, entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "registerDefaultEntitlementModule", entitlementModule)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) RegisterDefaultEntitlementModule(entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RegisterDefaultEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, entitlementModule)
}

// RegisterDefaultEntitlementModule is a paid mutator transaction binding the contract method 0xdbe83cbf.
//
// Solidity: function registerDefaultEntitlementModule(address entitlementModule) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) RegisterDefaultEntitlementModule(entitlementModule common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RegisterDefaultEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, entitlementModule)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0x1e984060.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint256[] roleIds, bytes data) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) RemoveEntitlement(opts *bind.TransactOpts, spaceId *big.Int, entitlementModuleAddress common.Address, roleIds []*big.Int, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "removeEntitlement", spaceId, entitlementModuleAddress, roleIds, data)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0x1e984060.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint256[] roleIds, bytes data) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) RemoveEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, roleIds []*big.Int, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RemoveEntitlement(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementModuleAddress, roleIds, data)
}

// RemoveEntitlement is a paid mutator transaction binding the contract method 0x1e984060.
//
// Solidity: function removeEntitlement(uint256 spaceId, address entitlementModuleAddress, uint256[] roleIds, bytes data) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) RemoveEntitlement(spaceId *big.Int, entitlementModuleAddress common.Address, roleIds []*big.Int, data []byte) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RemoveEntitlement(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementModuleAddress, roleIds, data)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) RenounceOwnership() (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RenounceOwnership(&_ZionSpaceManagerGoerli.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.RenounceOwnership(&_ZionSpaceManagerGoerli.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.TransferOwnership(&_ZionSpaceManagerGoerli.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.TransferOwnership(&_ZionSpaceManagerGoerli.TransactOpts, newOwner)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactor) WhitelistEntitlementModule(opts *bind.TransactOpts, spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.contract.Transact(opts, "whitelistEntitlementModule", spaceId, entitlementAddress, whitelist)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliSession) WhitelistEntitlementModule(spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.WhitelistEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementAddress, whitelist)
}

// WhitelistEntitlementModule is a paid mutator transaction binding the contract method 0x28dcb202.
//
// Solidity: function whitelistEntitlementModule(uint256 spaceId, address entitlementAddress, bool whitelist) returns()
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliTransactorSession) WhitelistEntitlementModule(spaceId *big.Int, entitlementAddress common.Address, whitelist bool) (*types.Transaction, error) {
	return _ZionSpaceManagerGoerli.Contract.WhitelistEntitlementModule(&_ZionSpaceManagerGoerli.TransactOpts, spaceId, entitlementAddress, whitelist)
}

// ZionSpaceManagerGoerliOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the ZionSpaceManagerGoerli contract.
type ZionSpaceManagerGoerliOwnershipTransferredIterator struct {
	Event *ZionSpaceManagerGoerliOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *ZionSpaceManagerGoerliOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ZionSpaceManagerGoerliOwnershipTransferred)
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
		it.Event = new(ZionSpaceManagerGoerliOwnershipTransferred)
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
func (it *ZionSpaceManagerGoerliOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ZionSpaceManagerGoerliOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ZionSpaceManagerGoerliOwnershipTransferred represents a OwnershipTransferred event raised by the ZionSpaceManagerGoerli contract.
type ZionSpaceManagerGoerliOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*ZionSpaceManagerGoerliOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ZionSpaceManagerGoerli.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &ZionSpaceManagerGoerliOwnershipTransferredIterator{contract: _ZionSpaceManagerGoerli.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *ZionSpaceManagerGoerliOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ZionSpaceManagerGoerli.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ZionSpaceManagerGoerliOwnershipTransferred)
				if err := _ZionSpaceManagerGoerli.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_ZionSpaceManagerGoerli *ZionSpaceManagerGoerliFilterer) ParseOwnershipTransferred(log types.Log) (*ZionSpaceManagerGoerliOwnershipTransferred, error) {
	event := new(ZionSpaceManagerGoerliOwnershipTransferred)
	if err := _ZionSpaceManagerGoerli.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
