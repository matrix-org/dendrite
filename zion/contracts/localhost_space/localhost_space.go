// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package localhost_space

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

// DataTypesChannel is an auto generated low-level Go binding around an user-defined struct.
type DataTypesChannel struct {
	Name             string
	ChannelNetworkId string
	ChannelHash      [32]byte
	CreatedAt        *big.Int
	Disabled         bool
}

// DataTypesEntitlement is an auto generated low-level Go binding around an user-defined struct.
type DataTypesEntitlement struct {
	Module common.Address
	Data   []byte
}

// DataTypesRole is an auto generated low-level Go binding around an user-defined struct.
type DataTypesRole struct {
	RoleId *big.Int
	Name   string
}

// LocalhostSpaceMetaData contains all meta data concerning the LocalhostSpace contract.
var LocalhostSpaceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"AddRoleFailed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ChannelAlreadyRegistered\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ChannelDoesNotExist\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementAlreadyExists\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementAlreadyWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementModuleNotSupported\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EntitlementNotWhitelisted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidParameters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"MissingOwnerPermission\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameContainsInvalidCharacters\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NameLengthInvalid\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotAllowed\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"PermissionAlreadyExists\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RoleDoesNotExist\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"RoleIsAssignedToEntitlement\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"previousAdmin\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"newAdmin\",\"type\":\"address\"}],\"name\":\"AdminChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"beacon\",\"type\":\"address\"}],\"name\":\"BeaconUpgraded\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"version\",\"type\":\"uint8\"}],\"name\":\"Initialized\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"implementation\",\"type\":\"address\"}],\"name\":\"Upgraded\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"},{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"}],\"name\":\"addPermissionsToRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"_entitlement\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"addRoleToChannel\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"module\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structDataTypes.Entitlement\",\"name\":\"_entitlement\",\"type\":\"tuple\"}],\"name\":\"addRoleToEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"channels\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"channelsByHash\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"bytes32\",\"name\":\"channelHash\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"disabled\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"channelName\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"uint256[]\",\"name\":\"roleIds\",\"type\":\"uint256[]\"}],\"name\":\"createChannel\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_roleName\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"_permissions\",\"type\":\"string[]\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"module\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structDataTypes.Entitlement[]\",\"name\":\"_entitlements\",\"type\":\"tuple[]\"}],\"name\":\"createRole\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"defaultEntitlements\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"disabled\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"entitlements\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"_channelHash\",\"type\":\"bytes32\"}],\"name\":\"getChannelByHash\",\"outputs\":[{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"bytes32\",\"name\":\"channelHash\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"createdAt\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"disabled\",\"type\":\"bool\"}],\"internalType\":\"structDataTypes.Channel\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getChannels\",\"outputs\":[{\"internalType\":\"bytes32[]\",\"name\":\"\",\"type\":\"bytes32[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_moduleType\",\"type\":\"string\"}],\"name\":\"getEntitlementByModuleType\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"getEntitlementIdsByRoleId\",\"outputs\":[{\"internalType\":\"bytes32[]\",\"name\":\"\",\"type\":\"bytes32[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getEntitlements\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"getPermissionsByRoleId\",\"outputs\":[{\"internalType\":\"bytes32[]\",\"name\":\"\",\"type\":\"bytes32[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"getRoleById\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Role\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getRoles\",\"outputs\":[{\"components\":[{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structDataTypes.Role[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"hasEntitlement\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"_networkId\",\"type\":\"string\"},{\"internalType\":\"address[]\",\"name\":\"_entitlements\",\"type\":\"address[]\"},{\"internalType\":\"address\",\"name\":\"_token\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_tokenId\",\"type\":\"uint256\"}],\"name\":\"initialize\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"_user\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"_permission\",\"type\":\"string\"}],\"name\":\"isEntitledToChannel\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"_entitled\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_user\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"_permission\",\"type\":\"string\"}],\"name\":\"isEntitledToSpace\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"_entitled\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes[]\",\"name\":\"data\",\"type\":\"bytes[]\"}],\"name\":\"multicall\",\"outputs\":[{\"internalType\":\"bytes[]\",\"name\":\"results\",\"type\":\"bytes[]\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"name\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"networkId\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"ownerRoleId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"proxiableUUID\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"_permission\",\"type\":\"string\"}],\"name\":\"removePermissionFromRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"removeRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"_entitlement\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"removeRoleFromChannel\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"module\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structDataTypes.Entitlement\",\"name\":\"_entitlement\",\"type\":\"tuple\"}],\"name\":\"removeRoleFromEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"roleCount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"rolesById\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"bool\",\"name\":\"disableChannel\",\"type\":\"bool\"}],\"name\":\"setChannelAccess\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_entitlementModule\",\"type\":\"address\"},{\"internalType\":\"bool\",\"name\":\"_whitelist\",\"type\":\"bool\"}],\"name\":\"setEntitlementModule\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"}],\"name\":\"setOwnerRoleId\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bool\",\"name\":\"_disabled\",\"type\":\"bool\"}],\"name\":\"setSpaceAccess\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"token\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"tokenId\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"channelNetworkId\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"channelName\",\"type\":\"string\"}],\"name\":\"updateChannel\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_roleId\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"_roleName\",\"type\":\"string\"}],\"name\":\"updateRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_entitlement\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_newEntitlement\",\"type\":\"address\"}],\"name\":\"upgradeEntitlement\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"}],\"name\":\"upgradeTo\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newImplementation\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"upgradeToAndCall\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"}]",
}

// LocalhostSpaceABI is the input ABI used to generate the binding from.
// Deprecated: Use LocalhostSpaceMetaData.ABI instead.
var LocalhostSpaceABI = LocalhostSpaceMetaData.ABI

// LocalhostSpace is an auto generated Go binding around an Ethereum contract.
type LocalhostSpace struct {
	LocalhostSpaceCaller     // Read-only binding to the contract
	LocalhostSpaceTransactor // Write-only binding to the contract
	LocalhostSpaceFilterer   // Log filterer for contract events
}

// LocalhostSpaceCaller is an auto generated read-only Go binding around an Ethereum contract.
type LocalhostSpaceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type LocalhostSpaceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type LocalhostSpaceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// LocalhostSpaceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type LocalhostSpaceSession struct {
	Contract     *LocalhostSpace   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// LocalhostSpaceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type LocalhostSpaceCallerSession struct {
	Contract *LocalhostSpaceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// LocalhostSpaceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type LocalhostSpaceTransactorSession struct {
	Contract     *LocalhostSpaceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// LocalhostSpaceRaw is an auto generated low-level Go binding around an Ethereum contract.
type LocalhostSpaceRaw struct {
	Contract *LocalhostSpace // Generic contract binding to access the raw methods on
}

// LocalhostSpaceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type LocalhostSpaceCallerRaw struct {
	Contract *LocalhostSpaceCaller // Generic read-only contract binding to access the raw methods on
}

// LocalhostSpaceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type LocalhostSpaceTransactorRaw struct {
	Contract *LocalhostSpaceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewLocalhostSpace creates a new instance of LocalhostSpace, bound to a specific deployed contract.
func NewLocalhostSpace(address common.Address, backend bind.ContractBackend) (*LocalhostSpace, error) {
	contract, err := bindLocalhostSpace(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpace{LocalhostSpaceCaller: LocalhostSpaceCaller{contract: contract}, LocalhostSpaceTransactor: LocalhostSpaceTransactor{contract: contract}, LocalhostSpaceFilterer: LocalhostSpaceFilterer{contract: contract}}, nil
}

// NewLocalhostSpaceCaller creates a new read-only instance of LocalhostSpace, bound to a specific deployed contract.
func NewLocalhostSpaceCaller(address common.Address, caller bind.ContractCaller) (*LocalhostSpaceCaller, error) {
	contract, err := bindLocalhostSpace(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceCaller{contract: contract}, nil
}

// NewLocalhostSpaceTransactor creates a new write-only instance of LocalhostSpace, bound to a specific deployed contract.
func NewLocalhostSpaceTransactor(address common.Address, transactor bind.ContractTransactor) (*LocalhostSpaceTransactor, error) {
	contract, err := bindLocalhostSpace(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceTransactor{contract: contract}, nil
}

// NewLocalhostSpaceFilterer creates a new log filterer instance of LocalhostSpace, bound to a specific deployed contract.
func NewLocalhostSpaceFilterer(address common.Address, filterer bind.ContractFilterer) (*LocalhostSpaceFilterer, error) {
	contract, err := bindLocalhostSpace(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceFilterer{contract: contract}, nil
}

// bindLocalhostSpace binds a generic wrapper to an already deployed contract.
func bindLocalhostSpace(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(LocalhostSpaceABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LocalhostSpace *LocalhostSpaceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LocalhostSpace.Contract.LocalhostSpaceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LocalhostSpace *LocalhostSpaceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.LocalhostSpaceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LocalhostSpace *LocalhostSpaceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.LocalhostSpaceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_LocalhostSpace *LocalhostSpaceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _LocalhostSpace.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_LocalhostSpace *LocalhostSpaceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_LocalhostSpace *LocalhostSpaceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.contract.Transact(opts, method, params...)
}

// Channels is a free data retrieval call binding the contract method 0xe5949b5d.
//
// Solidity: function channels(uint256 ) view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceCaller) Channels(opts *bind.CallOpts, arg0 *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "channels", arg0)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// Channels is a free data retrieval call binding the contract method 0xe5949b5d.
//
// Solidity: function channels(uint256 ) view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceSession) Channels(arg0 *big.Int) ([32]byte, error) {
	return _LocalhostSpace.Contract.Channels(&_LocalhostSpace.CallOpts, arg0)
}

// Channels is a free data retrieval call binding the contract method 0xe5949b5d.
//
// Solidity: function channels(uint256 ) view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Channels(arg0 *big.Int) ([32]byte, error) {
	return _LocalhostSpace.Contract.Channels(&_LocalhostSpace.CallOpts, arg0)
}

// ChannelsByHash is a free data retrieval call binding the contract method 0x129ab3c8.
//
// Solidity: function channelsByHash(bytes32 ) view returns(string name, string channelNetworkId, bytes32 channelHash, uint256 createdAt, bool disabled)
func (_LocalhostSpace *LocalhostSpaceCaller) ChannelsByHash(opts *bind.CallOpts, arg0 [32]byte) (struct {
	Name             string
	ChannelNetworkId string
	ChannelHash      [32]byte
	CreatedAt        *big.Int
	Disabled         bool
}, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "channelsByHash", arg0)

	outstruct := new(struct {
		Name             string
		ChannelNetworkId string
		ChannelHash      [32]byte
		CreatedAt        *big.Int
		Disabled         bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Name = *abi.ConvertType(out[0], new(string)).(*string)
	outstruct.ChannelNetworkId = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.ChannelHash = *abi.ConvertType(out[2], new([32]byte)).(*[32]byte)
	outstruct.CreatedAt = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.Disabled = *abi.ConvertType(out[4], new(bool)).(*bool)

	return *outstruct, err

}

// ChannelsByHash is a free data retrieval call binding the contract method 0x129ab3c8.
//
// Solidity: function channelsByHash(bytes32 ) view returns(string name, string channelNetworkId, bytes32 channelHash, uint256 createdAt, bool disabled)
func (_LocalhostSpace *LocalhostSpaceSession) ChannelsByHash(arg0 [32]byte) (struct {
	Name             string
	ChannelNetworkId string
	ChannelHash      [32]byte
	CreatedAt        *big.Int
	Disabled         bool
}, error) {
	return _LocalhostSpace.Contract.ChannelsByHash(&_LocalhostSpace.CallOpts, arg0)
}

// ChannelsByHash is a free data retrieval call binding the contract method 0x129ab3c8.
//
// Solidity: function channelsByHash(bytes32 ) view returns(string name, string channelNetworkId, bytes32 channelHash, uint256 createdAt, bool disabled)
func (_LocalhostSpace *LocalhostSpaceCallerSession) ChannelsByHash(arg0 [32]byte) (struct {
	Name             string
	ChannelNetworkId string
	ChannelHash      [32]byte
	CreatedAt        *big.Int
	Disabled         bool
}, error) {
	return _LocalhostSpace.Contract.ChannelsByHash(&_LocalhostSpace.CallOpts, arg0)
}

// DefaultEntitlements is a free data retrieval call binding the contract method 0xfa6433c5.
//
// Solidity: function defaultEntitlements(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCaller) DefaultEntitlements(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "defaultEntitlements", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// DefaultEntitlements is a free data retrieval call binding the contract method 0xfa6433c5.
//
// Solidity: function defaultEntitlements(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceSession) DefaultEntitlements(arg0 common.Address) (bool, error) {
	return _LocalhostSpace.Contract.DefaultEntitlements(&_LocalhostSpace.CallOpts, arg0)
}

// DefaultEntitlements is a free data retrieval call binding the contract method 0xfa6433c5.
//
// Solidity: function defaultEntitlements(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCallerSession) DefaultEntitlements(arg0 common.Address) (bool, error) {
	return _LocalhostSpace.Contract.DefaultEntitlements(&_LocalhostSpace.CallOpts, arg0)
}

// Disabled is a free data retrieval call binding the contract method 0xee070805.
//
// Solidity: function disabled() view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCaller) Disabled(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "disabled")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Disabled is a free data retrieval call binding the contract method 0xee070805.
//
// Solidity: function disabled() view returns(bool)
func (_LocalhostSpace *LocalhostSpaceSession) Disabled() (bool, error) {
	return _LocalhostSpace.Contract.Disabled(&_LocalhostSpace.CallOpts)
}

// Disabled is a free data retrieval call binding the contract method 0xee070805.
//
// Solidity: function disabled() view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Disabled() (bool, error) {
	return _LocalhostSpace.Contract.Disabled(&_LocalhostSpace.CallOpts)
}

// Entitlements is a free data retrieval call binding the contract method 0xf28f9b56.
//
// Solidity: function entitlements(uint256 ) view returns(address)
func (_LocalhostSpace *LocalhostSpaceCaller) Entitlements(opts *bind.CallOpts, arg0 *big.Int) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "entitlements", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Entitlements is a free data retrieval call binding the contract method 0xf28f9b56.
//
// Solidity: function entitlements(uint256 ) view returns(address)
func (_LocalhostSpace *LocalhostSpaceSession) Entitlements(arg0 *big.Int) (common.Address, error) {
	return _LocalhostSpace.Contract.Entitlements(&_LocalhostSpace.CallOpts, arg0)
}

// Entitlements is a free data retrieval call binding the contract method 0xf28f9b56.
//
// Solidity: function entitlements(uint256 ) view returns(address)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Entitlements(arg0 *big.Int) (common.Address, error) {
	return _LocalhostSpace.Contract.Entitlements(&_LocalhostSpace.CallOpts, arg0)
}

// GetChannelByHash is a free data retrieval call binding the contract method 0x703511f8.
//
// Solidity: function getChannelByHash(bytes32 _channelHash) view returns((string,string,bytes32,uint256,bool))
func (_LocalhostSpace *LocalhostSpaceCaller) GetChannelByHash(opts *bind.CallOpts, _channelHash [32]byte) (DataTypesChannel, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getChannelByHash", _channelHash)

	if err != nil {
		return *new(DataTypesChannel), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesChannel)).(*DataTypesChannel)

	return out0, err

}

// GetChannelByHash is a free data retrieval call binding the contract method 0x703511f8.
//
// Solidity: function getChannelByHash(bytes32 _channelHash) view returns((string,string,bytes32,uint256,bool))
func (_LocalhostSpace *LocalhostSpaceSession) GetChannelByHash(_channelHash [32]byte) (DataTypesChannel, error) {
	return _LocalhostSpace.Contract.GetChannelByHash(&_LocalhostSpace.CallOpts, _channelHash)
}

// GetChannelByHash is a free data retrieval call binding the contract method 0x703511f8.
//
// Solidity: function getChannelByHash(bytes32 _channelHash) view returns((string,string,bytes32,uint256,bool))
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetChannelByHash(_channelHash [32]byte) (DataTypesChannel, error) {
	return _LocalhostSpace.Contract.GetChannelByHash(&_LocalhostSpace.CallOpts, _channelHash)
}

// GetChannels is a free data retrieval call binding the contract method 0x9575f6ac.
//
// Solidity: function getChannels() view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCaller) GetChannels(opts *bind.CallOpts) ([][32]byte, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getChannels")

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// GetChannels is a free data retrieval call binding the contract method 0x9575f6ac.
//
// Solidity: function getChannels() view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceSession) GetChannels() ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetChannels(&_LocalhostSpace.CallOpts)
}

// GetChannels is a free data retrieval call binding the contract method 0x9575f6ac.
//
// Solidity: function getChannels() view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetChannels() ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetChannels(&_LocalhostSpace.CallOpts)
}

// GetEntitlementByModuleType is a free data retrieval call binding the contract method 0x870b9464.
//
// Solidity: function getEntitlementByModuleType(string _moduleType) view returns(address)
func (_LocalhostSpace *LocalhostSpaceCaller) GetEntitlementByModuleType(opts *bind.CallOpts, _moduleType string) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getEntitlementByModuleType", _moduleType)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetEntitlementByModuleType is a free data retrieval call binding the contract method 0x870b9464.
//
// Solidity: function getEntitlementByModuleType(string _moduleType) view returns(address)
func (_LocalhostSpace *LocalhostSpaceSession) GetEntitlementByModuleType(_moduleType string) (common.Address, error) {
	return _LocalhostSpace.Contract.GetEntitlementByModuleType(&_LocalhostSpace.CallOpts, _moduleType)
}

// GetEntitlementByModuleType is a free data retrieval call binding the contract method 0x870b9464.
//
// Solidity: function getEntitlementByModuleType(string _moduleType) view returns(address)
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetEntitlementByModuleType(_moduleType string) (common.Address, error) {
	return _LocalhostSpace.Contract.GetEntitlementByModuleType(&_LocalhostSpace.CallOpts, _moduleType)
}

// GetEntitlementIdsByRoleId is a free data retrieval call binding the contract method 0x42486e49.
//
// Solidity: function getEntitlementIdsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCaller) GetEntitlementIdsByRoleId(opts *bind.CallOpts, _roleId *big.Int) ([][32]byte, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getEntitlementIdsByRoleId", _roleId)

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// GetEntitlementIdsByRoleId is a free data retrieval call binding the contract method 0x42486e49.
//
// Solidity: function getEntitlementIdsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceSession) GetEntitlementIdsByRoleId(_roleId *big.Int) ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetEntitlementIdsByRoleId(&_LocalhostSpace.CallOpts, _roleId)
}

// GetEntitlementIdsByRoleId is a free data retrieval call binding the contract method 0x42486e49.
//
// Solidity: function getEntitlementIdsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetEntitlementIdsByRoleId(_roleId *big.Int) ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetEntitlementIdsByRoleId(&_LocalhostSpace.CallOpts, _roleId)
}

// GetEntitlements is a free data retrieval call binding the contract method 0x487dc38c.
//
// Solidity: function getEntitlements() view returns(address[])
func (_LocalhostSpace *LocalhostSpaceCaller) GetEntitlements(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getEntitlements")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetEntitlements is a free data retrieval call binding the contract method 0x487dc38c.
//
// Solidity: function getEntitlements() view returns(address[])
func (_LocalhostSpace *LocalhostSpaceSession) GetEntitlements() ([]common.Address, error) {
	return _LocalhostSpace.Contract.GetEntitlements(&_LocalhostSpace.CallOpts)
}

// GetEntitlements is a free data retrieval call binding the contract method 0x487dc38c.
//
// Solidity: function getEntitlements() view returns(address[])
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetEntitlements() ([]common.Address, error) {
	return _LocalhostSpace.Contract.GetEntitlements(&_LocalhostSpace.CallOpts)
}

// GetPermissionsByRoleId is a free data retrieval call binding the contract method 0xb4264233.
//
// Solidity: function getPermissionsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCaller) GetPermissionsByRoleId(opts *bind.CallOpts, _roleId *big.Int) ([][32]byte, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getPermissionsByRoleId", _roleId)

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// GetPermissionsByRoleId is a free data retrieval call binding the contract method 0xb4264233.
//
// Solidity: function getPermissionsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceSession) GetPermissionsByRoleId(_roleId *big.Int) ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetPermissionsByRoleId(&_LocalhostSpace.CallOpts, _roleId)
}

// GetPermissionsByRoleId is a free data retrieval call binding the contract method 0xb4264233.
//
// Solidity: function getPermissionsByRoleId(uint256 _roleId) view returns(bytes32[])
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetPermissionsByRoleId(_roleId *big.Int) ([][32]byte, error) {
	return _LocalhostSpace.Contract.GetPermissionsByRoleId(&_LocalhostSpace.CallOpts, _roleId)
}

// GetRoleById is a free data retrieval call binding the contract method 0x784c872b.
//
// Solidity: function getRoleById(uint256 _roleId) view returns((uint256,string))
func (_LocalhostSpace *LocalhostSpaceCaller) GetRoleById(opts *bind.CallOpts, _roleId *big.Int) (DataTypesRole, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getRoleById", _roleId)

	if err != nil {
		return *new(DataTypesRole), err
	}

	out0 := *abi.ConvertType(out[0], new(DataTypesRole)).(*DataTypesRole)

	return out0, err

}

// GetRoleById is a free data retrieval call binding the contract method 0x784c872b.
//
// Solidity: function getRoleById(uint256 _roleId) view returns((uint256,string))
func (_LocalhostSpace *LocalhostSpaceSession) GetRoleById(_roleId *big.Int) (DataTypesRole, error) {
	return _LocalhostSpace.Contract.GetRoleById(&_LocalhostSpace.CallOpts, _roleId)
}

// GetRoleById is a free data retrieval call binding the contract method 0x784c872b.
//
// Solidity: function getRoleById(uint256 _roleId) view returns((uint256,string))
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetRoleById(_roleId *big.Int) (DataTypesRole, error) {
	return _LocalhostSpace.Contract.GetRoleById(&_LocalhostSpace.CallOpts, _roleId)
}

// GetRoles is a free data retrieval call binding the contract method 0x71061398.
//
// Solidity: function getRoles() view returns((uint256,string)[])
func (_LocalhostSpace *LocalhostSpaceCaller) GetRoles(opts *bind.CallOpts) ([]DataTypesRole, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "getRoles")

	if err != nil {
		return *new([]DataTypesRole), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataTypesRole)).(*[]DataTypesRole)

	return out0, err

}

// GetRoles is a free data retrieval call binding the contract method 0x71061398.
//
// Solidity: function getRoles() view returns((uint256,string)[])
func (_LocalhostSpace *LocalhostSpaceSession) GetRoles() ([]DataTypesRole, error) {
	return _LocalhostSpace.Contract.GetRoles(&_LocalhostSpace.CallOpts)
}

// GetRoles is a free data retrieval call binding the contract method 0x71061398.
//
// Solidity: function getRoles() view returns((uint256,string)[])
func (_LocalhostSpace *LocalhostSpaceCallerSession) GetRoles() ([]DataTypesRole, error) {
	return _LocalhostSpace.Contract.GetRoles(&_LocalhostSpace.CallOpts)
}

// HasEntitlement is a free data retrieval call binding the contract method 0x7f8d06d0.
//
// Solidity: function hasEntitlement(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCaller) HasEntitlement(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "hasEntitlement", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// HasEntitlement is a free data retrieval call binding the contract method 0x7f8d06d0.
//
// Solidity: function hasEntitlement(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceSession) HasEntitlement(arg0 common.Address) (bool, error) {
	return _LocalhostSpace.Contract.HasEntitlement(&_LocalhostSpace.CallOpts, arg0)
}

// HasEntitlement is a free data retrieval call binding the contract method 0x7f8d06d0.
//
// Solidity: function hasEntitlement(address ) view returns(bool)
func (_LocalhostSpace *LocalhostSpaceCallerSession) HasEntitlement(arg0 common.Address) (bool, error) {
	return _LocalhostSpace.Contract.HasEntitlement(&_LocalhostSpace.CallOpts, arg0)
}

// IsEntitledToChannel is a free data retrieval call binding the contract method 0xcea632bc.
//
// Solidity: function isEntitledToChannel(string _channelNetworkId, address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceCaller) IsEntitledToChannel(opts *bind.CallOpts, _channelNetworkId string, _user common.Address, _permission string) (bool, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "isEntitledToChannel", _channelNetworkId, _user, _permission)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitledToChannel is a free data retrieval call binding the contract method 0xcea632bc.
//
// Solidity: function isEntitledToChannel(string _channelNetworkId, address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceSession) IsEntitledToChannel(_channelNetworkId string, _user common.Address, _permission string) (bool, error) {
	return _LocalhostSpace.Contract.IsEntitledToChannel(&_LocalhostSpace.CallOpts, _channelNetworkId, _user, _permission)
}

// IsEntitledToChannel is a free data retrieval call binding the contract method 0xcea632bc.
//
// Solidity: function isEntitledToChannel(string _channelNetworkId, address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceCallerSession) IsEntitledToChannel(_channelNetworkId string, _user common.Address, _permission string) (bool, error) {
	return _LocalhostSpace.Contract.IsEntitledToChannel(&_LocalhostSpace.CallOpts, _channelNetworkId, _user, _permission)
}

// IsEntitledToSpace is a free data retrieval call binding the contract method 0x20759f9e.
//
// Solidity: function isEntitledToSpace(address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceCaller) IsEntitledToSpace(opts *bind.CallOpts, _user common.Address, _permission string) (bool, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "isEntitledToSpace", _user, _permission)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsEntitledToSpace is a free data retrieval call binding the contract method 0x20759f9e.
//
// Solidity: function isEntitledToSpace(address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceSession) IsEntitledToSpace(_user common.Address, _permission string) (bool, error) {
	return _LocalhostSpace.Contract.IsEntitledToSpace(&_LocalhostSpace.CallOpts, _user, _permission)
}

// IsEntitledToSpace is a free data retrieval call binding the contract method 0x20759f9e.
//
// Solidity: function isEntitledToSpace(address _user, string _permission) view returns(bool _entitled)
func (_LocalhostSpace *LocalhostSpaceCallerSession) IsEntitledToSpace(_user common.Address, _permission string) (bool, error) {
	return _LocalhostSpace.Contract.IsEntitledToSpace(&_LocalhostSpace.CallOpts, _user, _permission)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_LocalhostSpace *LocalhostSpaceCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_LocalhostSpace *LocalhostSpaceSession) Name() (string, error) {
	return _LocalhostSpace.Contract.Name(&_LocalhostSpace.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Name() (string, error) {
	return _LocalhostSpace.Contract.Name(&_LocalhostSpace.CallOpts)
}

// NetworkId is a free data retrieval call binding the contract method 0x9025e64c.
//
// Solidity: function networkId() view returns(string)
func (_LocalhostSpace *LocalhostSpaceCaller) NetworkId(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "networkId")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// NetworkId is a free data retrieval call binding the contract method 0x9025e64c.
//
// Solidity: function networkId() view returns(string)
func (_LocalhostSpace *LocalhostSpaceSession) NetworkId() (string, error) {
	return _LocalhostSpace.Contract.NetworkId(&_LocalhostSpace.CallOpts)
}

// NetworkId is a free data retrieval call binding the contract method 0x9025e64c.
//
// Solidity: function networkId() view returns(string)
func (_LocalhostSpace *LocalhostSpaceCallerSession) NetworkId() (string, error) {
	return _LocalhostSpace.Contract.NetworkId(&_LocalhostSpace.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpace *LocalhostSpaceCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpace *LocalhostSpaceSession) Owner() (common.Address, error) {
	return _LocalhostSpace.Contract.Owner(&_LocalhostSpace.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Owner() (common.Address, error) {
	return _LocalhostSpace.Contract.Owner(&_LocalhostSpace.CallOpts)
}

// OwnerRoleId is a free data retrieval call binding the contract method 0xd1a6a961.
//
// Solidity: function ownerRoleId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCaller) OwnerRoleId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "ownerRoleId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// OwnerRoleId is a free data retrieval call binding the contract method 0xd1a6a961.
//
// Solidity: function ownerRoleId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceSession) OwnerRoleId() (*big.Int, error) {
	return _LocalhostSpace.Contract.OwnerRoleId(&_LocalhostSpace.CallOpts)
}

// OwnerRoleId is a free data retrieval call binding the contract method 0xd1a6a961.
//
// Solidity: function ownerRoleId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCallerSession) OwnerRoleId() (*big.Int, error) {
	return _LocalhostSpace.Contract.OwnerRoleId(&_LocalhostSpace.CallOpts)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceCaller) ProxiableUUID(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "proxiableUUID")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceSession) ProxiableUUID() ([32]byte, error) {
	return _LocalhostSpace.Contract.ProxiableUUID(&_LocalhostSpace.CallOpts)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceCallerSession) ProxiableUUID() ([32]byte, error) {
	return _LocalhostSpace.Contract.ProxiableUUID(&_LocalhostSpace.CallOpts)
}

// RoleCount is a free data retrieval call binding the contract method 0xddf96358.
//
// Solidity: function roleCount() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCaller) RoleCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "roleCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RoleCount is a free data retrieval call binding the contract method 0xddf96358.
//
// Solidity: function roleCount() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceSession) RoleCount() (*big.Int, error) {
	return _LocalhostSpace.Contract.RoleCount(&_LocalhostSpace.CallOpts)
}

// RoleCount is a free data retrieval call binding the contract method 0xddf96358.
//
// Solidity: function roleCount() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCallerSession) RoleCount() (*big.Int, error) {
	return _LocalhostSpace.Contract.RoleCount(&_LocalhostSpace.CallOpts)
}

// RolesById is a free data retrieval call binding the contract method 0xe5894ef4.
//
// Solidity: function rolesById(uint256 ) view returns(uint256 roleId, string name)
func (_LocalhostSpace *LocalhostSpaceCaller) RolesById(opts *bind.CallOpts, arg0 *big.Int) (struct {
	RoleId *big.Int
	Name   string
}, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "rolesById", arg0)

	outstruct := new(struct {
		RoleId *big.Int
		Name   string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.RoleId = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)

	return *outstruct, err

}

// RolesById is a free data retrieval call binding the contract method 0xe5894ef4.
//
// Solidity: function rolesById(uint256 ) view returns(uint256 roleId, string name)
func (_LocalhostSpace *LocalhostSpaceSession) RolesById(arg0 *big.Int) (struct {
	RoleId *big.Int
	Name   string
}, error) {
	return _LocalhostSpace.Contract.RolesById(&_LocalhostSpace.CallOpts, arg0)
}

// RolesById is a free data retrieval call binding the contract method 0xe5894ef4.
//
// Solidity: function rolesById(uint256 ) view returns(uint256 roleId, string name)
func (_LocalhostSpace *LocalhostSpaceCallerSession) RolesById(arg0 *big.Int) (struct {
	RoleId *big.Int
	Name   string
}, error) {
	return _LocalhostSpace.Contract.RolesById(&_LocalhostSpace.CallOpts, arg0)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_LocalhostSpace *LocalhostSpaceCaller) Token(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "token")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_LocalhostSpace *LocalhostSpaceSession) Token() (common.Address, error) {
	return _LocalhostSpace.Contract.Token(&_LocalhostSpace.CallOpts)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_LocalhostSpace *LocalhostSpaceCallerSession) Token() (common.Address, error) {
	return _LocalhostSpace.Contract.Token(&_LocalhostSpace.CallOpts)
}

// TokenId is a free data retrieval call binding the contract method 0x17d70f7c.
//
// Solidity: function tokenId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCaller) TokenId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _LocalhostSpace.contract.Call(opts, &out, "tokenId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TokenId is a free data retrieval call binding the contract method 0x17d70f7c.
//
// Solidity: function tokenId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceSession) TokenId() (*big.Int, error) {
	return _LocalhostSpace.Contract.TokenId(&_LocalhostSpace.CallOpts)
}

// TokenId is a free data retrieval call binding the contract method 0x17d70f7c.
//
// Solidity: function tokenId() view returns(uint256)
func (_LocalhostSpace *LocalhostSpaceCallerSession) TokenId() (*big.Int, error) {
	return _LocalhostSpace.Contract.TokenId(&_LocalhostSpace.CallOpts)
}

// AddPermissionsToRole is a paid mutator transaction binding the contract method 0xb7515761.
//
// Solidity: function addPermissionsToRole(uint256 _roleId, string[] _permissions) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) AddPermissionsToRole(opts *bind.TransactOpts, _roleId *big.Int, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "addPermissionsToRole", _roleId, _permissions)
}

// AddPermissionsToRole is a paid mutator transaction binding the contract method 0xb7515761.
//
// Solidity: function addPermissionsToRole(uint256 _roleId, string[] _permissions) returns()
func (_LocalhostSpace *LocalhostSpaceSession) AddPermissionsToRole(_roleId *big.Int, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddPermissionsToRole(&_LocalhostSpace.TransactOpts, _roleId, _permissions)
}

// AddPermissionsToRole is a paid mutator transaction binding the contract method 0xb7515761.
//
// Solidity: function addPermissionsToRole(uint256 _roleId, string[] _permissions) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) AddPermissionsToRole(_roleId *big.Int, _permissions []string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddPermissionsToRole(&_LocalhostSpace.TransactOpts, _roleId, _permissions)
}

// AddRoleToChannel is a paid mutator transaction binding the contract method 0x1dea616a.
//
// Solidity: function addRoleToChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) AddRoleToChannel(opts *bind.TransactOpts, _channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "addRoleToChannel", _channelNetworkId, _entitlement, _roleId)
}

// AddRoleToChannel is a paid mutator transaction binding the contract method 0x1dea616a.
//
// Solidity: function addRoleToChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceSession) AddRoleToChannel(_channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddRoleToChannel(&_LocalhostSpace.TransactOpts, _channelNetworkId, _entitlement, _roleId)
}

// AddRoleToChannel is a paid mutator transaction binding the contract method 0x1dea616a.
//
// Solidity: function addRoleToChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) AddRoleToChannel(_channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddRoleToChannel(&_LocalhostSpace.TransactOpts, _channelNetworkId, _entitlement, _roleId)
}

// AddRoleToEntitlement is a paid mutator transaction binding the contract method 0xba201ba8.
//
// Solidity: function addRoleToEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) AddRoleToEntitlement(opts *bind.TransactOpts, _roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "addRoleToEntitlement", _roleId, _entitlement)
}

// AddRoleToEntitlement is a paid mutator transaction binding the contract method 0xba201ba8.
//
// Solidity: function addRoleToEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceSession) AddRoleToEntitlement(_roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddRoleToEntitlement(&_LocalhostSpace.TransactOpts, _roleId, _entitlement)
}

// AddRoleToEntitlement is a paid mutator transaction binding the contract method 0xba201ba8.
//
// Solidity: function addRoleToEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) AddRoleToEntitlement(_roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.AddRoleToEntitlement(&_LocalhostSpace.TransactOpts, _roleId, _entitlement)
}

// CreateChannel is a paid mutator transaction binding the contract method 0x51f83cea.
//
// Solidity: function createChannel(string channelName, string channelNetworkId, uint256[] roleIds) returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceTransactor) CreateChannel(opts *bind.TransactOpts, channelName string, channelNetworkId string, roleIds []*big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "createChannel", channelName, channelNetworkId, roleIds)
}

// CreateChannel is a paid mutator transaction binding the contract method 0x51f83cea.
//
// Solidity: function createChannel(string channelName, string channelNetworkId, uint256[] roleIds) returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceSession) CreateChannel(channelName string, channelNetworkId string, roleIds []*big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.CreateChannel(&_LocalhostSpace.TransactOpts, channelName, channelNetworkId, roleIds)
}

// CreateChannel is a paid mutator transaction binding the contract method 0x51f83cea.
//
// Solidity: function createChannel(string channelName, string channelNetworkId, uint256[] roleIds) returns(bytes32)
func (_LocalhostSpace *LocalhostSpaceTransactorSession) CreateChannel(channelName string, channelNetworkId string, roleIds []*big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.CreateChannel(&_LocalhostSpace.TransactOpts, channelName, channelNetworkId, roleIds)
}

// CreateRole is a paid mutator transaction binding the contract method 0x8fcd793d.
//
// Solidity: function createRole(string _roleName, string[] _permissions, (address,bytes)[] _entitlements) returns(uint256)
func (_LocalhostSpace *LocalhostSpaceTransactor) CreateRole(opts *bind.TransactOpts, _roleName string, _permissions []string, _entitlements []DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "createRole", _roleName, _permissions, _entitlements)
}

// CreateRole is a paid mutator transaction binding the contract method 0x8fcd793d.
//
// Solidity: function createRole(string _roleName, string[] _permissions, (address,bytes)[] _entitlements) returns(uint256)
func (_LocalhostSpace *LocalhostSpaceSession) CreateRole(_roleName string, _permissions []string, _entitlements []DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.CreateRole(&_LocalhostSpace.TransactOpts, _roleName, _permissions, _entitlements)
}

// CreateRole is a paid mutator transaction binding the contract method 0x8fcd793d.
//
// Solidity: function createRole(string _roleName, string[] _permissions, (address,bytes)[] _entitlements) returns(uint256)
func (_LocalhostSpace *LocalhostSpaceTransactorSession) CreateRole(_roleName string, _permissions []string, _entitlements []DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.CreateRole(&_LocalhostSpace.TransactOpts, _roleName, _permissions, _entitlements)
}

// Initialize is a paid mutator transaction binding the contract method 0xf3ce6a5b.
//
// Solidity: function initialize(string _name, string _networkId, address[] _entitlements, address _token, uint256 _tokenId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) Initialize(opts *bind.TransactOpts, _name string, _networkId string, _entitlements []common.Address, _token common.Address, _tokenId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "initialize", _name, _networkId, _entitlements, _token, _tokenId)
}

// Initialize is a paid mutator transaction binding the contract method 0xf3ce6a5b.
//
// Solidity: function initialize(string _name, string _networkId, address[] _entitlements, address _token, uint256 _tokenId) returns()
func (_LocalhostSpace *LocalhostSpaceSession) Initialize(_name string, _networkId string, _entitlements []common.Address, _token common.Address, _tokenId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.Initialize(&_LocalhostSpace.TransactOpts, _name, _networkId, _entitlements, _token, _tokenId)
}

// Initialize is a paid mutator transaction binding the contract method 0xf3ce6a5b.
//
// Solidity: function initialize(string _name, string _networkId, address[] _entitlements, address _token, uint256 _tokenId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) Initialize(_name string, _networkId string, _entitlements []common.Address, _token common.Address, _tokenId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.Initialize(&_LocalhostSpace.TransactOpts, _name, _networkId, _entitlements, _token, _tokenId)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_LocalhostSpace *LocalhostSpaceTransactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_LocalhostSpace *LocalhostSpaceSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.Multicall(&_LocalhostSpace.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_LocalhostSpace *LocalhostSpaceTransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.Multicall(&_LocalhostSpace.TransactOpts, data)
}

// RemovePermissionFromRole is a paid mutator transaction binding the contract method 0xf740bb6b.
//
// Solidity: function removePermissionFromRole(uint256 _roleId, string _permission) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) RemovePermissionFromRole(opts *bind.TransactOpts, _roleId *big.Int, _permission string) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "removePermissionFromRole", _roleId, _permission)
}

// RemovePermissionFromRole is a paid mutator transaction binding the contract method 0xf740bb6b.
//
// Solidity: function removePermissionFromRole(uint256 _roleId, string _permission) returns()
func (_LocalhostSpace *LocalhostSpaceSession) RemovePermissionFromRole(_roleId *big.Int, _permission string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemovePermissionFromRole(&_LocalhostSpace.TransactOpts, _roleId, _permission)
}

// RemovePermissionFromRole is a paid mutator transaction binding the contract method 0xf740bb6b.
//
// Solidity: function removePermissionFromRole(uint256 _roleId, string _permission) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) RemovePermissionFromRole(_roleId *big.Int, _permission string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemovePermissionFromRole(&_LocalhostSpace.TransactOpts, _roleId, _permission)
}

// RemoveRole is a paid mutator transaction binding the contract method 0x92691821.
//
// Solidity: function removeRole(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) RemoveRole(opts *bind.TransactOpts, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "removeRole", _roleId)
}

// RemoveRole is a paid mutator transaction binding the contract method 0x92691821.
//
// Solidity: function removeRole(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceSession) RemoveRole(_roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRole(&_LocalhostSpace.TransactOpts, _roleId)
}

// RemoveRole is a paid mutator transaction binding the contract method 0x92691821.
//
// Solidity: function removeRole(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) RemoveRole(_roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRole(&_LocalhostSpace.TransactOpts, _roleId)
}

// RemoveRoleFromChannel is a paid mutator transaction binding the contract method 0xbaaf3d57.
//
// Solidity: function removeRoleFromChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) RemoveRoleFromChannel(opts *bind.TransactOpts, _channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "removeRoleFromChannel", _channelNetworkId, _entitlement, _roleId)
}

// RemoveRoleFromChannel is a paid mutator transaction binding the contract method 0xbaaf3d57.
//
// Solidity: function removeRoleFromChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceSession) RemoveRoleFromChannel(_channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRoleFromChannel(&_LocalhostSpace.TransactOpts, _channelNetworkId, _entitlement, _roleId)
}

// RemoveRoleFromChannel is a paid mutator transaction binding the contract method 0xbaaf3d57.
//
// Solidity: function removeRoleFromChannel(string _channelNetworkId, address _entitlement, uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) RemoveRoleFromChannel(_channelNetworkId string, _entitlement common.Address, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRoleFromChannel(&_LocalhostSpace.TransactOpts, _channelNetworkId, _entitlement, _roleId)
}

// RemoveRoleFromEntitlement is a paid mutator transaction binding the contract method 0xdba81864.
//
// Solidity: function removeRoleFromEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) RemoveRoleFromEntitlement(opts *bind.TransactOpts, _roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "removeRoleFromEntitlement", _roleId, _entitlement)
}

// RemoveRoleFromEntitlement is a paid mutator transaction binding the contract method 0xdba81864.
//
// Solidity: function removeRoleFromEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceSession) RemoveRoleFromEntitlement(_roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRoleFromEntitlement(&_LocalhostSpace.TransactOpts, _roleId, _entitlement)
}

// RemoveRoleFromEntitlement is a paid mutator transaction binding the contract method 0xdba81864.
//
// Solidity: function removeRoleFromEntitlement(uint256 _roleId, (address,bytes) _entitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) RemoveRoleFromEntitlement(_roleId *big.Int, _entitlement DataTypesEntitlement) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.RemoveRoleFromEntitlement(&_LocalhostSpace.TransactOpts, _roleId, _entitlement)
}

// SetChannelAccess is a paid mutator transaction binding the contract method 0x5de151b8.
//
// Solidity: function setChannelAccess(string channelNetworkId, bool disableChannel) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) SetChannelAccess(opts *bind.TransactOpts, channelNetworkId string, disableChannel bool) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "setChannelAccess", channelNetworkId, disableChannel)
}

// SetChannelAccess is a paid mutator transaction binding the contract method 0x5de151b8.
//
// Solidity: function setChannelAccess(string channelNetworkId, bool disableChannel) returns()
func (_LocalhostSpace *LocalhostSpaceSession) SetChannelAccess(channelNetworkId string, disableChannel bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetChannelAccess(&_LocalhostSpace.TransactOpts, channelNetworkId, disableChannel)
}

// SetChannelAccess is a paid mutator transaction binding the contract method 0x5de151b8.
//
// Solidity: function setChannelAccess(string channelNetworkId, bool disableChannel) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) SetChannelAccess(channelNetworkId string, disableChannel bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetChannelAccess(&_LocalhostSpace.TransactOpts, channelNetworkId, disableChannel)
}

// SetEntitlementModule is a paid mutator transaction binding the contract method 0x441555e5.
//
// Solidity: function setEntitlementModule(address _entitlementModule, bool _whitelist) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) SetEntitlementModule(opts *bind.TransactOpts, _entitlementModule common.Address, _whitelist bool) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "setEntitlementModule", _entitlementModule, _whitelist)
}

// SetEntitlementModule is a paid mutator transaction binding the contract method 0x441555e5.
//
// Solidity: function setEntitlementModule(address _entitlementModule, bool _whitelist) returns()
func (_LocalhostSpace *LocalhostSpaceSession) SetEntitlementModule(_entitlementModule common.Address, _whitelist bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetEntitlementModule(&_LocalhostSpace.TransactOpts, _entitlementModule, _whitelist)
}

// SetEntitlementModule is a paid mutator transaction binding the contract method 0x441555e5.
//
// Solidity: function setEntitlementModule(address _entitlementModule, bool _whitelist) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) SetEntitlementModule(_entitlementModule common.Address, _whitelist bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetEntitlementModule(&_LocalhostSpace.TransactOpts, _entitlementModule, _whitelist)
}

// SetOwnerRoleId is a paid mutator transaction binding the contract method 0x4999ab16.
//
// Solidity: function setOwnerRoleId(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) SetOwnerRoleId(opts *bind.TransactOpts, _roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "setOwnerRoleId", _roleId)
}

// SetOwnerRoleId is a paid mutator transaction binding the contract method 0x4999ab16.
//
// Solidity: function setOwnerRoleId(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceSession) SetOwnerRoleId(_roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetOwnerRoleId(&_LocalhostSpace.TransactOpts, _roleId)
}

// SetOwnerRoleId is a paid mutator transaction binding the contract method 0x4999ab16.
//
// Solidity: function setOwnerRoleId(uint256 _roleId) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) SetOwnerRoleId(_roleId *big.Int) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetOwnerRoleId(&_LocalhostSpace.TransactOpts, _roleId)
}

// SetSpaceAccess is a paid mutator transaction binding the contract method 0x446dc22e.
//
// Solidity: function setSpaceAccess(bool _disabled) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) SetSpaceAccess(opts *bind.TransactOpts, _disabled bool) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "setSpaceAccess", _disabled)
}

// SetSpaceAccess is a paid mutator transaction binding the contract method 0x446dc22e.
//
// Solidity: function setSpaceAccess(bool _disabled) returns()
func (_LocalhostSpace *LocalhostSpaceSession) SetSpaceAccess(_disabled bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetSpaceAccess(&_LocalhostSpace.TransactOpts, _disabled)
}

// SetSpaceAccess is a paid mutator transaction binding the contract method 0x446dc22e.
//
// Solidity: function setSpaceAccess(bool _disabled) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) SetSpaceAccess(_disabled bool) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.SetSpaceAccess(&_LocalhostSpace.TransactOpts, _disabled)
}

// UpdateChannel is a paid mutator transaction binding the contract method 0x34a1dd26.
//
// Solidity: function updateChannel(string channelNetworkId, string channelName) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) UpdateChannel(opts *bind.TransactOpts, channelNetworkId string, channelName string) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "updateChannel", channelNetworkId, channelName)
}

// UpdateChannel is a paid mutator transaction binding the contract method 0x34a1dd26.
//
// Solidity: function updateChannel(string channelNetworkId, string channelName) returns()
func (_LocalhostSpace *LocalhostSpaceSession) UpdateChannel(channelNetworkId string, channelName string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpdateChannel(&_LocalhostSpace.TransactOpts, channelNetworkId, channelName)
}

// UpdateChannel is a paid mutator transaction binding the contract method 0x34a1dd26.
//
// Solidity: function updateChannel(string channelNetworkId, string channelName) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) UpdateChannel(channelNetworkId string, channelName string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpdateChannel(&_LocalhostSpace.TransactOpts, channelNetworkId, channelName)
}

// UpdateRole is a paid mutator transaction binding the contract method 0x32e704cc.
//
// Solidity: function updateRole(uint256 _roleId, string _roleName) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) UpdateRole(opts *bind.TransactOpts, _roleId *big.Int, _roleName string) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "updateRole", _roleId, _roleName)
}

// UpdateRole is a paid mutator transaction binding the contract method 0x32e704cc.
//
// Solidity: function updateRole(uint256 _roleId, string _roleName) returns()
func (_LocalhostSpace *LocalhostSpaceSession) UpdateRole(_roleId *big.Int, _roleName string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpdateRole(&_LocalhostSpace.TransactOpts, _roleId, _roleName)
}

// UpdateRole is a paid mutator transaction binding the contract method 0x32e704cc.
//
// Solidity: function updateRole(uint256 _roleId, string _roleName) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) UpdateRole(_roleId *big.Int, _roleName string) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpdateRole(&_LocalhostSpace.TransactOpts, _roleId, _roleName)
}

// UpgradeEntitlement is a paid mutator transaction binding the contract method 0x519607f2.
//
// Solidity: function upgradeEntitlement(address _entitlement, address _newEntitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) UpgradeEntitlement(opts *bind.TransactOpts, _entitlement common.Address, _newEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "upgradeEntitlement", _entitlement, _newEntitlement)
}

// UpgradeEntitlement is a paid mutator transaction binding the contract method 0x519607f2.
//
// Solidity: function upgradeEntitlement(address _entitlement, address _newEntitlement) returns()
func (_LocalhostSpace *LocalhostSpaceSession) UpgradeEntitlement(_entitlement common.Address, _newEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeEntitlement(&_LocalhostSpace.TransactOpts, _entitlement, _newEntitlement)
}

// UpgradeEntitlement is a paid mutator transaction binding the contract method 0x519607f2.
//
// Solidity: function upgradeEntitlement(address _entitlement, address _newEntitlement) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) UpgradeEntitlement(_entitlement common.Address, _newEntitlement common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeEntitlement(&_LocalhostSpace.TransactOpts, _entitlement, _newEntitlement)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) UpgradeTo(opts *bind.TransactOpts, newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "upgradeTo", newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpace *LocalhostSpaceSession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeTo(&_LocalhostSpace.TransactOpts, newImplementation)
}

// UpgradeTo is a paid mutator transaction binding the contract method 0x3659cfe6.
//
// Solidity: function upgradeTo(address newImplementation) returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) UpgradeTo(newImplementation common.Address) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeTo(&_LocalhostSpace.TransactOpts, newImplementation)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpace *LocalhostSpaceTransactor) UpgradeToAndCall(opts *bind.TransactOpts, newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpace.contract.Transact(opts, "upgradeToAndCall", newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpace *LocalhostSpaceSession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeToAndCall(&_LocalhostSpace.TransactOpts, newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_LocalhostSpace *LocalhostSpaceTransactorSession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _LocalhostSpace.Contract.UpgradeToAndCall(&_LocalhostSpace.TransactOpts, newImplementation, data)
}

// LocalhostSpaceAdminChangedIterator is returned from FilterAdminChanged and is used to iterate over the raw logs and unpacked data for AdminChanged events raised by the LocalhostSpace contract.
type LocalhostSpaceAdminChangedIterator struct {
	Event *LocalhostSpaceAdminChanged // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceAdminChanged)
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
		it.Event = new(LocalhostSpaceAdminChanged)
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
func (it *LocalhostSpaceAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceAdminChanged represents a AdminChanged event raised by the LocalhostSpace contract.
type LocalhostSpaceAdminChanged struct {
	PreviousAdmin common.Address
	NewAdmin      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterAdminChanged is a free log retrieval operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_LocalhostSpace *LocalhostSpaceFilterer) FilterAdminChanged(opts *bind.FilterOpts) (*LocalhostSpaceAdminChangedIterator, error) {

	logs, sub, err := _LocalhostSpace.contract.FilterLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceAdminChangedIterator{contract: _LocalhostSpace.contract, event: "AdminChanged", logs: logs, sub: sub}, nil
}

// WatchAdminChanged is a free log subscription operation binding the contract event 0x7e644d79422f17c01e4894b5f4f588d331ebfa28653d42ae832dc59e38c9798f.
//
// Solidity: event AdminChanged(address previousAdmin, address newAdmin)
func (_LocalhostSpace *LocalhostSpaceFilterer) WatchAdminChanged(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceAdminChanged) (event.Subscription, error) {

	logs, sub, err := _LocalhostSpace.contract.WatchLogs(opts, "AdminChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceAdminChanged)
				if err := _LocalhostSpace.contract.UnpackLog(event, "AdminChanged", log); err != nil {
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
func (_LocalhostSpace *LocalhostSpaceFilterer) ParseAdminChanged(log types.Log) (*LocalhostSpaceAdminChanged, error) {
	event := new(LocalhostSpaceAdminChanged)
	if err := _LocalhostSpace.contract.UnpackLog(event, "AdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceBeaconUpgradedIterator is returned from FilterBeaconUpgraded and is used to iterate over the raw logs and unpacked data for BeaconUpgraded events raised by the LocalhostSpace contract.
type LocalhostSpaceBeaconUpgradedIterator struct {
	Event *LocalhostSpaceBeaconUpgraded // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceBeaconUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceBeaconUpgraded)
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
		it.Event = new(LocalhostSpaceBeaconUpgraded)
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
func (it *LocalhostSpaceBeaconUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceBeaconUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceBeaconUpgraded represents a BeaconUpgraded event raised by the LocalhostSpace contract.
type LocalhostSpaceBeaconUpgraded struct {
	Beacon common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterBeaconUpgraded is a free log retrieval operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_LocalhostSpace *LocalhostSpaceFilterer) FilterBeaconUpgraded(opts *bind.FilterOpts, beacon []common.Address) (*LocalhostSpaceBeaconUpgradedIterator, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _LocalhostSpace.contract.FilterLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceBeaconUpgradedIterator{contract: _LocalhostSpace.contract, event: "BeaconUpgraded", logs: logs, sub: sub}, nil
}

// WatchBeaconUpgraded is a free log subscription operation binding the contract event 0x1cf3b03a6cf19fa2baba4df148e9dcabedea7f8a5c07840e207e5c089be95d3e.
//
// Solidity: event BeaconUpgraded(address indexed beacon)
func (_LocalhostSpace *LocalhostSpaceFilterer) WatchBeaconUpgraded(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceBeaconUpgraded, beacon []common.Address) (event.Subscription, error) {

	var beaconRule []interface{}
	for _, beaconItem := range beacon {
		beaconRule = append(beaconRule, beaconItem)
	}

	logs, sub, err := _LocalhostSpace.contract.WatchLogs(opts, "BeaconUpgraded", beaconRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceBeaconUpgraded)
				if err := _LocalhostSpace.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
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
func (_LocalhostSpace *LocalhostSpaceFilterer) ParseBeaconUpgraded(log types.Log) (*LocalhostSpaceBeaconUpgraded, error) {
	event := new(LocalhostSpaceBeaconUpgraded)
	if err := _LocalhostSpace.contract.UnpackLog(event, "BeaconUpgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the LocalhostSpace contract.
type LocalhostSpaceInitializedIterator struct {
	Event *LocalhostSpaceInitialized // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceInitialized)
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
		it.Event = new(LocalhostSpaceInitialized)
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
func (it *LocalhostSpaceInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceInitialized represents a Initialized event raised by the LocalhostSpace contract.
type LocalhostSpaceInitialized struct {
	Version uint8
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_LocalhostSpace *LocalhostSpaceFilterer) FilterInitialized(opts *bind.FilterOpts) (*LocalhostSpaceInitializedIterator, error) {

	logs, sub, err := _LocalhostSpace.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceInitializedIterator{contract: _LocalhostSpace.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0x7f26b83ff96e1f2b6a682f133852f6798a09c465da95921460cefb3847402498.
//
// Solidity: event Initialized(uint8 version)
func (_LocalhostSpace *LocalhostSpaceFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceInitialized) (event.Subscription, error) {

	logs, sub, err := _LocalhostSpace.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceInitialized)
				if err := _LocalhostSpace.contract.UnpackLog(event, "Initialized", log); err != nil {
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
func (_LocalhostSpace *LocalhostSpaceFilterer) ParseInitialized(log types.Log) (*LocalhostSpaceInitialized, error) {
	event := new(LocalhostSpaceInitialized)
	if err := _LocalhostSpace.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// LocalhostSpaceUpgradedIterator is returned from FilterUpgraded and is used to iterate over the raw logs and unpacked data for Upgraded events raised by the LocalhostSpace contract.
type LocalhostSpaceUpgradedIterator struct {
	Event *LocalhostSpaceUpgraded // Event containing the contract specifics and raw log

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
func (it *LocalhostSpaceUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(LocalhostSpaceUpgraded)
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
		it.Event = new(LocalhostSpaceUpgraded)
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
func (it *LocalhostSpaceUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *LocalhostSpaceUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// LocalhostSpaceUpgraded represents a Upgraded event raised by the LocalhostSpace contract.
type LocalhostSpaceUpgraded struct {
	Implementation common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgraded is a free log retrieval operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_LocalhostSpace *LocalhostSpaceFilterer) FilterUpgraded(opts *bind.FilterOpts, implementation []common.Address) (*LocalhostSpaceUpgradedIterator, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _LocalhostSpace.contract.FilterLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return &LocalhostSpaceUpgradedIterator{contract: _LocalhostSpace.contract, event: "Upgraded", logs: logs, sub: sub}, nil
}

// WatchUpgraded is a free log subscription operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_LocalhostSpace *LocalhostSpaceFilterer) WatchUpgraded(opts *bind.WatchOpts, sink chan<- *LocalhostSpaceUpgraded, implementation []common.Address) (event.Subscription, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _LocalhostSpace.contract.WatchLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(LocalhostSpaceUpgraded)
				if err := _LocalhostSpace.contract.UnpackLog(event, "Upgraded", log); err != nil {
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
func (_LocalhostSpace *LocalhostSpaceFilterer) ParseUpgraded(log types.Log) (*LocalhostSpaceUpgraded, error) {
	event := new(LocalhostSpaceUpgraded)
	if err := _LocalhostSpace.contract.UnpackLog(event, "Upgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
