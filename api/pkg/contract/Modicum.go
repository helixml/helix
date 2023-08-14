// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contract

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
	_ = abi.ConvertType
)

// ModicumMetaData contains all meta data concerning the Modicum contract.
var ModicumMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"value\",\"type\":\"uint64\"}],\"name\":\"Debug\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"}],\"name\":\"DebugArch\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"string\",\"name\":\"str\",\"type\":\"string\"}],\"name\":\"DebugString\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"DebugUint\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"_from\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"enumModicum.EtherTransferCause\",\"name\":\"cause\",\"type\":\"uint8\"}],\"name\":\"EtherTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"}],\"name\":\"JobAssignedForMediation\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"JobCreatorAddedTrustedMediator\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"penaltyRate\",\"type\":\"uint256\"}],\"name\":\"JobCreatorRegistered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"offerId\",\"type\":\"uint256\"}],\"name\":\"JobOfferCanceled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"offerId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"ijoid\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionLimit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthLimit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionMaxPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthMaxPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"completionDeadline\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"deposit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchIncentive\",\"type\":\"uint256\"}],\"name\":\"JobOfferPostedPartOne\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"offerId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"hash\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"ramLimit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"localStorageLimit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"extras\",\"type\":\"string\"}],\"name\":\"JobOfferPostedPartTwo\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"cost\",\"type\":\"uint256\"}],\"name\":\"MatchClosed\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"jobOfferId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"resourceOfferId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"Matched\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"result\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"enumModicum.Party\",\"name\":\"faultyParty\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"enumModicum.Verdict\",\"name\":\"verdict\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"enumModicum.ResultStatus\",\"name\":\"status\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"hash\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionCount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"mediationCost\",\"type\":\"uint256\"}],\"name\":\"MediationResultPosted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"firstLayerHash\",\"type\":\"uint256\"}],\"name\":\"MediatorAddedSupportedFirstLayer\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"}],\"name\":\"MediatorAddedTrustedDirectory\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"availabilityValue\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"verificationCount\",\"type\":\"uint256\"}],\"name\":\"MediatorRegistered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"resOfferId\",\"type\":\"uint256\"}],\"name\":\"ResourceOfferCanceled\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"offerId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionCap\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"memoryCap\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"localStorageCap\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthCap\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthPrice\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"deposit\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"iroid\",\"type\":\"uint256\"}],\"name\":\"ResourceOfferPosted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"firstLayer\",\"type\":\"uint256\"}],\"name\":\"ResourceProviderAddedSupportedFirstLayer\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"}],\"name\":\"ResourceProviderAddedTrustedDirectory\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"ResourceProviderAddedTrustedMediator\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"timePerInstruction\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"penaltyRate\",\"type\":\"uint256\"}],\"name\":\"ResourceProviderRegistered\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"resultId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"enumModicum.ResultStatus\",\"name\":\"status\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"string\",\"name\":\"hash\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"instructionCount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"bandwidthUsage\",\"type\":\"uint256\"}],\"name\":\"ResultPosted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"resultId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"ResultReaction\",\"type\":\"uint256\"}],\"name\":\"ResultReaction\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"penaltyRate\",\"type\":\"uint256\"}],\"name\":\"penaltyRateSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"reactionDeadline\",\"type\":\"uint256\"}],\"name\":\"reactionDeadlineSet\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"resultId\",\"type\":\"uint256\"}],\"name\":\"acceptResult\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"}],\"name\":\"check\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"name\":\"getModuleCost\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"template\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"params\",\"type\":\"string\"}],\"name\":\"getModuleSpec\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getRequiredResourceProviderDeposit\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"jobCreatorAddTrustedMediator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"firstLayerHash\",\"type\":\"uint256\"}],\"name\":\"mediatorAddSupportedFirstLayer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"}],\"name\":\"mediatorAddTrustedDirectory\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"mediators\",\"outputs\":[{\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"internalType\":\"uint256\",\"name\":\"instructionPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"availabilityValue\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"verificationCount\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"moduleName\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"ijoid\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"instructionLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"instructionMaxPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthMaxPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"completionDeadline\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"matchIncentive\",\"type\":\"uint256\"}],\"name\":\"postJobOfferPartOne\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"ijoid\",\"type\":\"uint256\"},{\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"jobHash\",\"type\":\"uint256\"},{\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"extras\",\"type\":\"string\"}],\"name\":\"postJobOfferPartTwo\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"jobOfferId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"resourceOfferId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"postMatch\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"jobOfferId\",\"type\":\"uint256\"},{\"internalType\":\"enumModicum.ResultStatus\",\"name\":\"status\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"hash\",\"type\":\"string\"},{\"internalType\":\"enumModicum.Verdict\",\"name\":\"verdict\",\"type\":\"uint8\"},{\"internalType\":\"enumModicum.Party\",\"name\":\"faultyParty\",\"type\":\"uint8\"}],\"name\":\"postMediationResult\",\"outputs\":[{\"internalType\":\"enumModicum.Party\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint32\",\"name\":\"instructionPrice\",\"type\":\"uint32\"},{\"internalType\":\"uint256\",\"name\":\"instructionCap\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"memoryCap\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"localStorageCap\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthCap\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"matchIncentive\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"verificationCount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"misc\",\"type\":\"uint256\"}],\"name\":\"postResOffer\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"matchId\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"jobOfferId\",\"type\":\"uint256\"},{\"internalType\":\"enumModicum.ResultStatus\",\"name\":\"status\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"uri\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"hash\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"instructionCount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthUsage\",\"type\":\"uint256\"}],\"name\":\"postResult\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"registerJobCreator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"internalType\":\"uint256\",\"name\":\"instructionPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"bandwidthPrice\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"availabilityValue\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"verificationCount\",\"type\":\"uint256\"}],\"name\":\"registerMediator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumModicum.Architecture\",\"name\":\"arch\",\"type\":\"uint8\"},{\"internalType\":\"uint256\",\"name\":\"timePerInstruction\",\"type\":\"uint256\"}],\"name\":\"registerResourceProvider\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"resultId\",\"type\":\"uint256\"}],\"name\":\"rejectResult\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"firstLayerHash\",\"type\":\"uint256\"}],\"name\":\"resourceProviderAddSupportedFirstLayer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"directory\",\"type\":\"address\"}],\"name\":\"resourceProviderAddTrustedDirectory\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"mediator\",\"type\":\"address\"}],\"name\":\"resourceProviderAddTrustedMediator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"params\",\"type\":\"string\"},{\"internalType\":\"address[]\",\"name\":\"_mediators\",\"type\":\"address[]\"}],\"name\":\"runModule\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"params\",\"type\":\"string\"}],\"name\":\"runModuleWithDefaultMediators\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address[]\",\"name\":\"_defaultMediators\",\"type\":\"address[]\"}],\"name\":\"setDefaultMediators\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"cost\",\"type\":\"uint256\"}],\"name\":\"setModuleCost\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_penaltyRate\",\"type\":\"uint256\"}],\"name\":\"setPenaltyRate\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_reactionDeadline\",\"type\":\"uint256\"}],\"name\":\"setReactionDeadline\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"multiple\",\"type\":\"uint256\"}],\"name\":\"setResourceProviderDepositMultiple\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_myString\",\"type\":\"string\"},{\"internalType\":\"string[]\",\"name\":\"_arr\",\"type\":\"string[]\"}],\"name\":\"stringExists\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"test\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// ModicumABI is the input ABI used to generate the binding from.
// Deprecated: Use ModicumMetaData.ABI instead.
var ModicumABI = ModicumMetaData.ABI

// Modicum is an auto generated Go binding around an Ethereum contract.
type Modicum struct {
	ModicumCaller     // Read-only binding to the contract
	ModicumTransactor // Write-only binding to the contract
	ModicumFilterer   // Log filterer for contract events
}

// ModicumCaller is an auto generated read-only Go binding around an Ethereum contract.
type ModicumCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ModicumTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ModicumTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ModicumFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ModicumFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ModicumSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ModicumSession struct {
	Contract     *Modicum          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ModicumCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ModicumCallerSession struct {
	Contract *ModicumCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// ModicumTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ModicumTransactorSession struct {
	Contract     *ModicumTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// ModicumRaw is an auto generated low-level Go binding around an Ethereum contract.
type ModicumRaw struct {
	Contract *Modicum // Generic contract binding to access the raw methods on
}

// ModicumCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ModicumCallerRaw struct {
	Contract *ModicumCaller // Generic read-only contract binding to access the raw methods on
}

// ModicumTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ModicumTransactorRaw struct {
	Contract *ModicumTransactor // Generic write-only contract binding to access the raw methods on
}

// NewModicum creates a new instance of Modicum, bound to a specific deployed contract.
func NewModicum(address common.Address, backend bind.ContractBackend) (*Modicum, error) {
	contract, err := bindModicum(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Modicum{ModicumCaller: ModicumCaller{contract: contract}, ModicumTransactor: ModicumTransactor{contract: contract}, ModicumFilterer: ModicumFilterer{contract: contract}}, nil
}

// NewModicumCaller creates a new read-only instance of Modicum, bound to a specific deployed contract.
func NewModicumCaller(address common.Address, caller bind.ContractCaller) (*ModicumCaller, error) {
	contract, err := bindModicum(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ModicumCaller{contract: contract}, nil
}

// NewModicumTransactor creates a new write-only instance of Modicum, bound to a specific deployed contract.
func NewModicumTransactor(address common.Address, transactor bind.ContractTransactor) (*ModicumTransactor, error) {
	contract, err := bindModicum(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ModicumTransactor{contract: contract}, nil
}

// NewModicumFilterer creates a new log filterer instance of Modicum, bound to a specific deployed contract.
func NewModicumFilterer(address common.Address, filterer bind.ContractFilterer) (*ModicumFilterer, error) {
	contract, err := bindModicum(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ModicumFilterer{contract: contract}, nil
}

// bindModicum binds a generic wrapper to an already deployed contract.
func bindModicum(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ModicumMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Modicum *ModicumRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Modicum.Contract.ModicumCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Modicum *ModicumRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Modicum.Contract.ModicumTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Modicum *ModicumRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Modicum.Contract.ModicumTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Modicum *ModicumCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Modicum.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Modicum *ModicumTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Modicum.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Modicum *ModicumTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Modicum.Contract.contract.Transact(opts, method, params...)
}

// GetModuleCost is a free data retrieval call binding the contract method 0xd863e416.
//
// Solidity: function getModuleCost(string name) view returns(uint256)
func (_Modicum *ModicumCaller) GetModuleCost(opts *bind.CallOpts, name string) (*big.Int, error) {
	var out []interface{}
	err := _Modicum.contract.Call(opts, &out, "getModuleCost", name)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetModuleCost is a free data retrieval call binding the contract method 0xd863e416.
//
// Solidity: function getModuleCost(string name) view returns(uint256)
func (_Modicum *ModicumSession) GetModuleCost(name string) (*big.Int, error) {
	return _Modicum.Contract.GetModuleCost(&_Modicum.CallOpts, name)
}

// GetModuleCost is a free data retrieval call binding the contract method 0xd863e416.
//
// Solidity: function getModuleCost(string name) view returns(uint256)
func (_Modicum *ModicumCallerSession) GetModuleCost(name string) (*big.Int, error) {
	return _Modicum.Contract.GetModuleCost(&_Modicum.CallOpts, name)
}

// GetModuleSpec is a free data retrieval call binding the contract method 0xc5f223bf.
//
// Solidity: function getModuleSpec(string template, string params) pure returns(string)
func (_Modicum *ModicumCaller) GetModuleSpec(opts *bind.CallOpts, template string, params string) (string, error) {
	var out []interface{}
	err := _Modicum.contract.Call(opts, &out, "getModuleSpec", template, params)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// GetModuleSpec is a free data retrieval call binding the contract method 0xc5f223bf.
//
// Solidity: function getModuleSpec(string template, string params) pure returns(string)
func (_Modicum *ModicumSession) GetModuleSpec(template string, params string) (string, error) {
	return _Modicum.Contract.GetModuleSpec(&_Modicum.CallOpts, template, params)
}

// GetModuleSpec is a free data retrieval call binding the contract method 0xc5f223bf.
//
// Solidity: function getModuleSpec(string template, string params) pure returns(string)
func (_Modicum *ModicumCallerSession) GetModuleSpec(template string, params string) (string, error) {
	return _Modicum.Contract.GetModuleSpec(&_Modicum.CallOpts, template, params)
}

// GetRequiredResourceProviderDeposit is a free data retrieval call binding the contract method 0xfd83a431.
//
// Solidity: function getRequiredResourceProviderDeposit() view returns(uint256)
func (_Modicum *ModicumCaller) GetRequiredResourceProviderDeposit(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Modicum.contract.Call(opts, &out, "getRequiredResourceProviderDeposit")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetRequiredResourceProviderDeposit is a free data retrieval call binding the contract method 0xfd83a431.
//
// Solidity: function getRequiredResourceProviderDeposit() view returns(uint256)
func (_Modicum *ModicumSession) GetRequiredResourceProviderDeposit() (*big.Int, error) {
	return _Modicum.Contract.GetRequiredResourceProviderDeposit(&_Modicum.CallOpts)
}

// GetRequiredResourceProviderDeposit is a free data retrieval call binding the contract method 0xfd83a431.
//
// Solidity: function getRequiredResourceProviderDeposit() view returns(uint256)
func (_Modicum *ModicumCallerSession) GetRequiredResourceProviderDeposit() (*big.Int, error) {
	return _Modicum.Contract.GetRequiredResourceProviderDeposit(&_Modicum.CallOpts)
}

// Mediators is a free data retrieval call binding the contract method 0xd5fb4e18.
//
// Solidity: function mediators(address ) view returns(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumCaller) Mediators(opts *bind.CallOpts, arg0 common.Address) (struct {
	Arch              uint8
	InstructionPrice  *big.Int
	BandwidthPrice    *big.Int
	AvailabilityValue *big.Int
	VerificationCount *big.Int
}, error) {
	var out []interface{}
	err := _Modicum.contract.Call(opts, &out, "mediators", arg0)

	outstruct := new(struct {
		Arch              uint8
		InstructionPrice  *big.Int
		BandwidthPrice    *big.Int
		AvailabilityValue *big.Int
		VerificationCount *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Arch = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.InstructionPrice = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.BandwidthPrice = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.AvailabilityValue = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerificationCount = *abi.ConvertType(out[4], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// Mediators is a free data retrieval call binding the contract method 0xd5fb4e18.
//
// Solidity: function mediators(address ) view returns(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumSession) Mediators(arg0 common.Address) (struct {
	Arch              uint8
	InstructionPrice  *big.Int
	BandwidthPrice    *big.Int
	AvailabilityValue *big.Int
	VerificationCount *big.Int
}, error) {
	return _Modicum.Contract.Mediators(&_Modicum.CallOpts, arg0)
}

// Mediators is a free data retrieval call binding the contract method 0xd5fb4e18.
//
// Solidity: function mediators(address ) view returns(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumCallerSession) Mediators(arg0 common.Address) (struct {
	Arch              uint8
	InstructionPrice  *big.Int
	BandwidthPrice    *big.Int
	AvailabilityValue *big.Int
	VerificationCount *big.Int
}, error) {
	return _Modicum.Contract.Mediators(&_Modicum.CallOpts, arg0)
}

// StringExists is a free data retrieval call binding the contract method 0xa93ab0b1.
//
// Solidity: function stringExists(string _myString, string[] _arr) pure returns(bool)
func (_Modicum *ModicumCaller) StringExists(opts *bind.CallOpts, _myString string, _arr []string) (bool, error) {
	var out []interface{}
	err := _Modicum.contract.Call(opts, &out, "stringExists", _myString, _arr)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// StringExists is a free data retrieval call binding the contract method 0xa93ab0b1.
//
// Solidity: function stringExists(string _myString, string[] _arr) pure returns(bool)
func (_Modicum *ModicumSession) StringExists(_myString string, _arr []string) (bool, error) {
	return _Modicum.Contract.StringExists(&_Modicum.CallOpts, _myString, _arr)
}

// StringExists is a free data retrieval call binding the contract method 0xa93ab0b1.
//
// Solidity: function stringExists(string _myString, string[] _arr) pure returns(bool)
func (_Modicum *ModicumCallerSession) StringExists(_myString string, _arr []string) (bool, error) {
	return _Modicum.Contract.StringExists(&_Modicum.CallOpts, _myString, _arr)
}

// AcceptResult is a paid mutator transaction binding the contract method 0x172257c7.
//
// Solidity: function acceptResult(uint256 resultId) returns(uint256)
func (_Modicum *ModicumTransactor) AcceptResult(opts *bind.TransactOpts, resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "acceptResult", resultId)
}

// AcceptResult is a paid mutator transaction binding the contract method 0x172257c7.
//
// Solidity: function acceptResult(uint256 resultId) returns(uint256)
func (_Modicum *ModicumSession) AcceptResult(resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.AcceptResult(&_Modicum.TransactOpts, resultId)
}

// AcceptResult is a paid mutator transaction binding the contract method 0x172257c7.
//
// Solidity: function acceptResult(uint256 resultId) returns(uint256)
func (_Modicum *ModicumTransactorSession) AcceptResult(resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.AcceptResult(&_Modicum.TransactOpts, resultId)
}

// Check is a paid mutator transaction binding the contract method 0x4fea39c2.
//
// Solidity: function check(uint8 arch) returns()
func (_Modicum *ModicumTransactor) Check(opts *bind.TransactOpts, arch uint8) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "check", arch)
}

// Check is a paid mutator transaction binding the contract method 0x4fea39c2.
//
// Solidity: function check(uint8 arch) returns()
func (_Modicum *ModicumSession) Check(arch uint8) (*types.Transaction, error) {
	return _Modicum.Contract.Check(&_Modicum.TransactOpts, arch)
}

// Check is a paid mutator transaction binding the contract method 0x4fea39c2.
//
// Solidity: function check(uint8 arch) returns()
func (_Modicum *ModicumTransactorSession) Check(arch uint8) (*types.Transaction, error) {
	return _Modicum.Contract.Check(&_Modicum.TransactOpts, arch)
}

// JobCreatorAddTrustedMediator is a paid mutator transaction binding the contract method 0x8a54fcee.
//
// Solidity: function jobCreatorAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumTransactor) JobCreatorAddTrustedMediator(opts *bind.TransactOpts, mediator common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "jobCreatorAddTrustedMediator", mediator)
}

// JobCreatorAddTrustedMediator is a paid mutator transaction binding the contract method 0x8a54fcee.
//
// Solidity: function jobCreatorAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumSession) JobCreatorAddTrustedMediator(mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.JobCreatorAddTrustedMediator(&_Modicum.TransactOpts, mediator)
}

// JobCreatorAddTrustedMediator is a paid mutator transaction binding the contract method 0x8a54fcee.
//
// Solidity: function jobCreatorAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumTransactorSession) JobCreatorAddTrustedMediator(mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.JobCreatorAddTrustedMediator(&_Modicum.TransactOpts, mediator)
}

// MediatorAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x413ba302.
//
// Solidity: function mediatorAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumTransactor) MediatorAddSupportedFirstLayer(opts *bind.TransactOpts, firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "mediatorAddSupportedFirstLayer", firstLayerHash)
}

// MediatorAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x413ba302.
//
// Solidity: function mediatorAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumSession) MediatorAddSupportedFirstLayer(firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.MediatorAddSupportedFirstLayer(&_Modicum.TransactOpts, firstLayerHash)
}

// MediatorAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x413ba302.
//
// Solidity: function mediatorAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumTransactorSession) MediatorAddSupportedFirstLayer(firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.MediatorAddSupportedFirstLayer(&_Modicum.TransactOpts, firstLayerHash)
}

// MediatorAddTrustedDirectory is a paid mutator transaction binding the contract method 0x82792410.
//
// Solidity: function mediatorAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumTransactor) MediatorAddTrustedDirectory(opts *bind.TransactOpts, directory common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "mediatorAddTrustedDirectory", directory)
}

// MediatorAddTrustedDirectory is a paid mutator transaction binding the contract method 0x82792410.
//
// Solidity: function mediatorAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumSession) MediatorAddTrustedDirectory(directory common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.MediatorAddTrustedDirectory(&_Modicum.TransactOpts, directory)
}

// MediatorAddTrustedDirectory is a paid mutator transaction binding the contract method 0x82792410.
//
// Solidity: function mediatorAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumTransactorSession) MediatorAddTrustedDirectory(directory common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.MediatorAddTrustedDirectory(&_Modicum.TransactOpts, directory)
}

// PostJobOfferPartOne is a paid mutator transaction binding the contract method 0x7857ef79.
//
// Solidity: function postJobOfferPartOne(string moduleName, uint256 ijoid, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 matchIncentive) payable returns(uint256)
func (_Modicum *ModicumTransactor) PostJobOfferPartOne(opts *bind.TransactOpts, moduleName string, ijoid *big.Int, instructionLimit *big.Int, bandwidthLimit *big.Int, instructionMaxPrice *big.Int, bandwidthMaxPrice *big.Int, completionDeadline *big.Int, matchIncentive *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postJobOfferPartOne", moduleName, ijoid, instructionLimit, bandwidthLimit, instructionMaxPrice, bandwidthMaxPrice, completionDeadline, matchIncentive)
}

// PostJobOfferPartOne is a paid mutator transaction binding the contract method 0x7857ef79.
//
// Solidity: function postJobOfferPartOne(string moduleName, uint256 ijoid, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 matchIncentive) payable returns(uint256)
func (_Modicum *ModicumSession) PostJobOfferPartOne(moduleName string, ijoid *big.Int, instructionLimit *big.Int, bandwidthLimit *big.Int, instructionMaxPrice *big.Int, bandwidthMaxPrice *big.Int, completionDeadline *big.Int, matchIncentive *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostJobOfferPartOne(&_Modicum.TransactOpts, moduleName, ijoid, instructionLimit, bandwidthLimit, instructionMaxPrice, bandwidthMaxPrice, completionDeadline, matchIncentive)
}

// PostJobOfferPartOne is a paid mutator transaction binding the contract method 0x7857ef79.
//
// Solidity: function postJobOfferPartOne(string moduleName, uint256 ijoid, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 matchIncentive) payable returns(uint256)
func (_Modicum *ModicumTransactorSession) PostJobOfferPartOne(moduleName string, ijoid *big.Int, instructionLimit *big.Int, bandwidthLimit *big.Int, instructionMaxPrice *big.Int, bandwidthMaxPrice *big.Int, completionDeadline *big.Int, matchIncentive *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostJobOfferPartOne(&_Modicum.TransactOpts, moduleName, ijoid, instructionLimit, bandwidthLimit, instructionMaxPrice, bandwidthMaxPrice, completionDeadline, matchIncentive)
}

// PostJobOfferPartTwo is a paid mutator transaction binding the contract method 0xa87fe3dd.
//
// Solidity: function postJobOfferPartTwo(uint256 ijoid, string uri, address directory, uint256 jobHash, uint8 arch, string extras) returns(uint256)
func (_Modicum *ModicumTransactor) PostJobOfferPartTwo(opts *bind.TransactOpts, ijoid *big.Int, uri string, directory common.Address, jobHash *big.Int, arch uint8, extras string) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postJobOfferPartTwo", ijoid, uri, directory, jobHash, arch, extras)
}

// PostJobOfferPartTwo is a paid mutator transaction binding the contract method 0xa87fe3dd.
//
// Solidity: function postJobOfferPartTwo(uint256 ijoid, string uri, address directory, uint256 jobHash, uint8 arch, string extras) returns(uint256)
func (_Modicum *ModicumSession) PostJobOfferPartTwo(ijoid *big.Int, uri string, directory common.Address, jobHash *big.Int, arch uint8, extras string) (*types.Transaction, error) {
	return _Modicum.Contract.PostJobOfferPartTwo(&_Modicum.TransactOpts, ijoid, uri, directory, jobHash, arch, extras)
}

// PostJobOfferPartTwo is a paid mutator transaction binding the contract method 0xa87fe3dd.
//
// Solidity: function postJobOfferPartTwo(uint256 ijoid, string uri, address directory, uint256 jobHash, uint8 arch, string extras) returns(uint256)
func (_Modicum *ModicumTransactorSession) PostJobOfferPartTwo(ijoid *big.Int, uri string, directory common.Address, jobHash *big.Int, arch uint8, extras string) (*types.Transaction, error) {
	return _Modicum.Contract.PostJobOfferPartTwo(&_Modicum.TransactOpts, ijoid, uri, directory, jobHash, arch, extras)
}

// PostMatch is a paid mutator transaction binding the contract method 0xff91ab04.
//
// Solidity: function postMatch(uint256 jobOfferId, uint256 resourceOfferId, address mediator) returns(uint256)
func (_Modicum *ModicumTransactor) PostMatch(opts *bind.TransactOpts, jobOfferId *big.Int, resourceOfferId *big.Int, mediator common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postMatch", jobOfferId, resourceOfferId, mediator)
}

// PostMatch is a paid mutator transaction binding the contract method 0xff91ab04.
//
// Solidity: function postMatch(uint256 jobOfferId, uint256 resourceOfferId, address mediator) returns(uint256)
func (_Modicum *ModicumSession) PostMatch(jobOfferId *big.Int, resourceOfferId *big.Int, mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.PostMatch(&_Modicum.TransactOpts, jobOfferId, resourceOfferId, mediator)
}

// PostMatch is a paid mutator transaction binding the contract method 0xff91ab04.
//
// Solidity: function postMatch(uint256 jobOfferId, uint256 resourceOfferId, address mediator) returns(uint256)
func (_Modicum *ModicumTransactorSession) PostMatch(jobOfferId *big.Int, resourceOfferId *big.Int, mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.PostMatch(&_Modicum.TransactOpts, jobOfferId, resourceOfferId, mediator)
}

// PostMediationResult is a paid mutator transaction binding the contract method 0x64f66c71.
//
// Solidity: function postMediationResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint8 verdict, uint8 faultyParty) returns(uint8)
func (_Modicum *ModicumTransactor) PostMediationResult(opts *bind.TransactOpts, matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, verdict uint8, faultyParty uint8) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postMediationResult", matchId, jobOfferId, status, uri, hash, verdict, faultyParty)
}

// PostMediationResult is a paid mutator transaction binding the contract method 0x64f66c71.
//
// Solidity: function postMediationResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint8 verdict, uint8 faultyParty) returns(uint8)
func (_Modicum *ModicumSession) PostMediationResult(matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, verdict uint8, faultyParty uint8) (*types.Transaction, error) {
	return _Modicum.Contract.PostMediationResult(&_Modicum.TransactOpts, matchId, jobOfferId, status, uri, hash, verdict, faultyParty)
}

// PostMediationResult is a paid mutator transaction binding the contract method 0x64f66c71.
//
// Solidity: function postMediationResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint8 verdict, uint8 faultyParty) returns(uint8)
func (_Modicum *ModicumTransactorSession) PostMediationResult(matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, verdict uint8, faultyParty uint8) (*types.Transaction, error) {
	return _Modicum.Contract.PostMediationResult(&_Modicum.TransactOpts, matchId, jobOfferId, status, uri, hash, verdict, faultyParty)
}

// PostResOffer is a paid mutator transaction binding the contract method 0x6a15da92.
//
// Solidity: function postResOffer(uint32 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 matchIncentive, uint256 verificationCount, uint256 misc) payable returns()
func (_Modicum *ModicumTransactor) PostResOffer(opts *bind.TransactOpts, instructionPrice uint32, instructionCap *big.Int, memoryCap *big.Int, localStorageCap *big.Int, bandwidthCap *big.Int, bandwidthPrice *big.Int, matchIncentive *big.Int, verificationCount *big.Int, misc *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postResOffer", instructionPrice, instructionCap, memoryCap, localStorageCap, bandwidthCap, bandwidthPrice, matchIncentive, verificationCount, misc)
}

// PostResOffer is a paid mutator transaction binding the contract method 0x6a15da92.
//
// Solidity: function postResOffer(uint32 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 matchIncentive, uint256 verificationCount, uint256 misc) payable returns()
func (_Modicum *ModicumSession) PostResOffer(instructionPrice uint32, instructionCap *big.Int, memoryCap *big.Int, localStorageCap *big.Int, bandwidthCap *big.Int, bandwidthPrice *big.Int, matchIncentive *big.Int, verificationCount *big.Int, misc *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostResOffer(&_Modicum.TransactOpts, instructionPrice, instructionCap, memoryCap, localStorageCap, bandwidthCap, bandwidthPrice, matchIncentive, verificationCount, misc)
}

// PostResOffer is a paid mutator transaction binding the contract method 0x6a15da92.
//
// Solidity: function postResOffer(uint32 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 matchIncentive, uint256 verificationCount, uint256 misc) payable returns()
func (_Modicum *ModicumTransactorSession) PostResOffer(instructionPrice uint32, instructionCap *big.Int, memoryCap *big.Int, localStorageCap *big.Int, bandwidthCap *big.Int, bandwidthPrice *big.Int, matchIncentive *big.Int, verificationCount *big.Int, misc *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostResOffer(&_Modicum.TransactOpts, instructionPrice, instructionCap, memoryCap, localStorageCap, bandwidthCap, bandwidthPrice, matchIncentive, verificationCount, misc)
}

// PostResult is a paid mutator transaction binding the contract method 0x1af8f8c5.
//
// Solidity: function postResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage) returns(uint256)
func (_Modicum *ModicumTransactor) PostResult(opts *bind.TransactOpts, matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, instructionCount *big.Int, bandwidthUsage *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "postResult", matchId, jobOfferId, status, uri, hash, instructionCount, bandwidthUsage)
}

// PostResult is a paid mutator transaction binding the contract method 0x1af8f8c5.
//
// Solidity: function postResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage) returns(uint256)
func (_Modicum *ModicumSession) PostResult(matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, instructionCount *big.Int, bandwidthUsage *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostResult(&_Modicum.TransactOpts, matchId, jobOfferId, status, uri, hash, instructionCount, bandwidthUsage)
}

// PostResult is a paid mutator transaction binding the contract method 0x1af8f8c5.
//
// Solidity: function postResult(uint256 matchId, uint256 jobOfferId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage) returns(uint256)
func (_Modicum *ModicumTransactorSession) PostResult(matchId *big.Int, jobOfferId *big.Int, status uint8, uri string, hash string, instructionCount *big.Int, bandwidthUsage *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.PostResult(&_Modicum.TransactOpts, matchId, jobOfferId, status, uri, hash, instructionCount, bandwidthUsage)
}

// RegisterJobCreator is a paid mutator transaction binding the contract method 0xc1a1668f.
//
// Solidity: function registerJobCreator() returns()
func (_Modicum *ModicumTransactor) RegisterJobCreator(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "registerJobCreator")
}

// RegisterJobCreator is a paid mutator transaction binding the contract method 0xc1a1668f.
//
// Solidity: function registerJobCreator() returns()
func (_Modicum *ModicumSession) RegisterJobCreator() (*types.Transaction, error) {
	return _Modicum.Contract.RegisterJobCreator(&_Modicum.TransactOpts)
}

// RegisterJobCreator is a paid mutator transaction binding the contract method 0xc1a1668f.
//
// Solidity: function registerJobCreator() returns()
func (_Modicum *ModicumTransactorSession) RegisterJobCreator() (*types.Transaction, error) {
	return _Modicum.Contract.RegisterJobCreator(&_Modicum.TransactOpts)
}

// RegisterMediator is a paid mutator transaction binding the contract method 0x0d4722f6.
//
// Solidity: function registerMediator(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount) returns()
func (_Modicum *ModicumTransactor) RegisterMediator(opts *bind.TransactOpts, arch uint8, instructionPrice *big.Int, bandwidthPrice *big.Int, availabilityValue *big.Int, verificationCount *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "registerMediator", arch, instructionPrice, bandwidthPrice, availabilityValue, verificationCount)
}

// RegisterMediator is a paid mutator transaction binding the contract method 0x0d4722f6.
//
// Solidity: function registerMediator(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount) returns()
func (_Modicum *ModicumSession) RegisterMediator(arch uint8, instructionPrice *big.Int, bandwidthPrice *big.Int, availabilityValue *big.Int, verificationCount *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RegisterMediator(&_Modicum.TransactOpts, arch, instructionPrice, bandwidthPrice, availabilityValue, verificationCount)
}

// RegisterMediator is a paid mutator transaction binding the contract method 0x0d4722f6.
//
// Solidity: function registerMediator(uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount) returns()
func (_Modicum *ModicumTransactorSession) RegisterMediator(arch uint8, instructionPrice *big.Int, bandwidthPrice *big.Int, availabilityValue *big.Int, verificationCount *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RegisterMediator(&_Modicum.TransactOpts, arch, instructionPrice, bandwidthPrice, availabilityValue, verificationCount)
}

// RegisterResourceProvider is a paid mutator transaction binding the contract method 0x75158b70.
//
// Solidity: function registerResourceProvider(uint8 arch, uint256 timePerInstruction) returns()
func (_Modicum *ModicumTransactor) RegisterResourceProvider(opts *bind.TransactOpts, arch uint8, timePerInstruction *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "registerResourceProvider", arch, timePerInstruction)
}

// RegisterResourceProvider is a paid mutator transaction binding the contract method 0x75158b70.
//
// Solidity: function registerResourceProvider(uint8 arch, uint256 timePerInstruction) returns()
func (_Modicum *ModicumSession) RegisterResourceProvider(arch uint8, timePerInstruction *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RegisterResourceProvider(&_Modicum.TransactOpts, arch, timePerInstruction)
}

// RegisterResourceProvider is a paid mutator transaction binding the contract method 0x75158b70.
//
// Solidity: function registerResourceProvider(uint8 arch, uint256 timePerInstruction) returns()
func (_Modicum *ModicumTransactorSession) RegisterResourceProvider(arch uint8, timePerInstruction *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RegisterResourceProvider(&_Modicum.TransactOpts, arch, timePerInstruction)
}

// RejectResult is a paid mutator transaction binding the contract method 0xdfc85219.
//
// Solidity: function rejectResult(uint256 resultId) returns()
func (_Modicum *ModicumTransactor) RejectResult(opts *bind.TransactOpts, resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "rejectResult", resultId)
}

// RejectResult is a paid mutator transaction binding the contract method 0xdfc85219.
//
// Solidity: function rejectResult(uint256 resultId) returns()
func (_Modicum *ModicumSession) RejectResult(resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RejectResult(&_Modicum.TransactOpts, resultId)
}

// RejectResult is a paid mutator transaction binding the contract method 0xdfc85219.
//
// Solidity: function rejectResult(uint256 resultId) returns()
func (_Modicum *ModicumTransactorSession) RejectResult(resultId *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.RejectResult(&_Modicum.TransactOpts, resultId)
}

// ResourceProviderAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x41591992.
//
// Solidity: function resourceProviderAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumTransactor) ResourceProviderAddSupportedFirstLayer(opts *bind.TransactOpts, firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "resourceProviderAddSupportedFirstLayer", firstLayerHash)
}

// ResourceProviderAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x41591992.
//
// Solidity: function resourceProviderAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumSession) ResourceProviderAddSupportedFirstLayer(firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddSupportedFirstLayer(&_Modicum.TransactOpts, firstLayerHash)
}

// ResourceProviderAddSupportedFirstLayer is a paid mutator transaction binding the contract method 0x41591992.
//
// Solidity: function resourceProviderAddSupportedFirstLayer(uint256 firstLayerHash) returns()
func (_Modicum *ModicumTransactorSession) ResourceProviderAddSupportedFirstLayer(firstLayerHash *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddSupportedFirstLayer(&_Modicum.TransactOpts, firstLayerHash)
}

// ResourceProviderAddTrustedDirectory is a paid mutator transaction binding the contract method 0xcf72e7bd.
//
// Solidity: function resourceProviderAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumTransactor) ResourceProviderAddTrustedDirectory(opts *bind.TransactOpts, directory common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "resourceProviderAddTrustedDirectory", directory)
}

// ResourceProviderAddTrustedDirectory is a paid mutator transaction binding the contract method 0xcf72e7bd.
//
// Solidity: function resourceProviderAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumSession) ResourceProviderAddTrustedDirectory(directory common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddTrustedDirectory(&_Modicum.TransactOpts, directory)
}

// ResourceProviderAddTrustedDirectory is a paid mutator transaction binding the contract method 0xcf72e7bd.
//
// Solidity: function resourceProviderAddTrustedDirectory(address directory) returns()
func (_Modicum *ModicumTransactorSession) ResourceProviderAddTrustedDirectory(directory common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddTrustedDirectory(&_Modicum.TransactOpts, directory)
}

// ResourceProviderAddTrustedMediator is a paid mutator transaction binding the contract method 0x2245ae9e.
//
// Solidity: function resourceProviderAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumTransactor) ResourceProviderAddTrustedMediator(opts *bind.TransactOpts, mediator common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "resourceProviderAddTrustedMediator", mediator)
}

// ResourceProviderAddTrustedMediator is a paid mutator transaction binding the contract method 0x2245ae9e.
//
// Solidity: function resourceProviderAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumSession) ResourceProviderAddTrustedMediator(mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddTrustedMediator(&_Modicum.TransactOpts, mediator)
}

// ResourceProviderAddTrustedMediator is a paid mutator transaction binding the contract method 0x2245ae9e.
//
// Solidity: function resourceProviderAddTrustedMediator(address mediator) returns()
func (_Modicum *ModicumTransactorSession) ResourceProviderAddTrustedMediator(mediator common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.ResourceProviderAddTrustedMediator(&_Modicum.TransactOpts, mediator)
}

// RunModule is a paid mutator transaction binding the contract method 0xa797f1c7.
//
// Solidity: function runModule(string name, string params, address[] _mediators) payable returns(uint256)
func (_Modicum *ModicumTransactor) RunModule(opts *bind.TransactOpts, name string, params string, _mediators []common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "runModule", name, params, _mediators)
}

// RunModule is a paid mutator transaction binding the contract method 0xa797f1c7.
//
// Solidity: function runModule(string name, string params, address[] _mediators) payable returns(uint256)
func (_Modicum *ModicumSession) RunModule(name string, params string, _mediators []common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.RunModule(&_Modicum.TransactOpts, name, params, _mediators)
}

// RunModule is a paid mutator transaction binding the contract method 0xa797f1c7.
//
// Solidity: function runModule(string name, string params, address[] _mediators) payable returns(uint256)
func (_Modicum *ModicumTransactorSession) RunModule(name string, params string, _mediators []common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.RunModule(&_Modicum.TransactOpts, name, params, _mediators)
}

// RunModuleWithDefaultMediators is a paid mutator transaction binding the contract method 0x7ce7b179.
//
// Solidity: function runModuleWithDefaultMediators(string name, string params) payable returns(uint256)
func (_Modicum *ModicumTransactor) RunModuleWithDefaultMediators(opts *bind.TransactOpts, name string, params string) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "runModuleWithDefaultMediators", name, params)
}

// RunModuleWithDefaultMediators is a paid mutator transaction binding the contract method 0x7ce7b179.
//
// Solidity: function runModuleWithDefaultMediators(string name, string params) payable returns(uint256)
func (_Modicum *ModicumSession) RunModuleWithDefaultMediators(name string, params string) (*types.Transaction, error) {
	return _Modicum.Contract.RunModuleWithDefaultMediators(&_Modicum.TransactOpts, name, params)
}

// RunModuleWithDefaultMediators is a paid mutator transaction binding the contract method 0x7ce7b179.
//
// Solidity: function runModuleWithDefaultMediators(string name, string params) payable returns(uint256)
func (_Modicum *ModicumTransactorSession) RunModuleWithDefaultMediators(name string, params string) (*types.Transaction, error) {
	return _Modicum.Contract.RunModuleWithDefaultMediators(&_Modicum.TransactOpts, name, params)
}

// SetDefaultMediators is a paid mutator transaction binding the contract method 0x87dbbb3d.
//
// Solidity: function setDefaultMediators(address[] _defaultMediators) returns()
func (_Modicum *ModicumTransactor) SetDefaultMediators(opts *bind.TransactOpts, _defaultMediators []common.Address) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "setDefaultMediators", _defaultMediators)
}

// SetDefaultMediators is a paid mutator transaction binding the contract method 0x87dbbb3d.
//
// Solidity: function setDefaultMediators(address[] _defaultMediators) returns()
func (_Modicum *ModicumSession) SetDefaultMediators(_defaultMediators []common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.SetDefaultMediators(&_Modicum.TransactOpts, _defaultMediators)
}

// SetDefaultMediators is a paid mutator transaction binding the contract method 0x87dbbb3d.
//
// Solidity: function setDefaultMediators(address[] _defaultMediators) returns()
func (_Modicum *ModicumTransactorSession) SetDefaultMediators(_defaultMediators []common.Address) (*types.Transaction, error) {
	return _Modicum.Contract.SetDefaultMediators(&_Modicum.TransactOpts, _defaultMediators)
}

// SetModuleCost is a paid mutator transaction binding the contract method 0x053d970c.
//
// Solidity: function setModuleCost(string name, uint256 cost) returns()
func (_Modicum *ModicumTransactor) SetModuleCost(opts *bind.TransactOpts, name string, cost *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "setModuleCost", name, cost)
}

// SetModuleCost is a paid mutator transaction binding the contract method 0x053d970c.
//
// Solidity: function setModuleCost(string name, uint256 cost) returns()
func (_Modicum *ModicumSession) SetModuleCost(name string, cost *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetModuleCost(&_Modicum.TransactOpts, name, cost)
}

// SetModuleCost is a paid mutator transaction binding the contract method 0x053d970c.
//
// Solidity: function setModuleCost(string name, uint256 cost) returns()
func (_Modicum *ModicumTransactorSession) SetModuleCost(name string, cost *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetModuleCost(&_Modicum.TransactOpts, name, cost)
}

// SetPenaltyRate is a paid mutator transaction binding the contract method 0xa1bab447.
//
// Solidity: function setPenaltyRate(uint256 _penaltyRate) returns()
func (_Modicum *ModicumTransactor) SetPenaltyRate(opts *bind.TransactOpts, _penaltyRate *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "setPenaltyRate", _penaltyRate)
}

// SetPenaltyRate is a paid mutator transaction binding the contract method 0xa1bab447.
//
// Solidity: function setPenaltyRate(uint256 _penaltyRate) returns()
func (_Modicum *ModicumSession) SetPenaltyRate(_penaltyRate *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetPenaltyRate(&_Modicum.TransactOpts, _penaltyRate)
}

// SetPenaltyRate is a paid mutator transaction binding the contract method 0xa1bab447.
//
// Solidity: function setPenaltyRate(uint256 _penaltyRate) returns()
func (_Modicum *ModicumTransactorSession) SetPenaltyRate(_penaltyRate *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetPenaltyRate(&_Modicum.TransactOpts, _penaltyRate)
}

// SetReactionDeadline is a paid mutator transaction binding the contract method 0x02ddffbe.
//
// Solidity: function setReactionDeadline(uint256 _reactionDeadline) returns()
func (_Modicum *ModicumTransactor) SetReactionDeadline(opts *bind.TransactOpts, _reactionDeadline *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "setReactionDeadline", _reactionDeadline)
}

// SetReactionDeadline is a paid mutator transaction binding the contract method 0x02ddffbe.
//
// Solidity: function setReactionDeadline(uint256 _reactionDeadline) returns()
func (_Modicum *ModicumSession) SetReactionDeadline(_reactionDeadline *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetReactionDeadline(&_Modicum.TransactOpts, _reactionDeadline)
}

// SetReactionDeadline is a paid mutator transaction binding the contract method 0x02ddffbe.
//
// Solidity: function setReactionDeadline(uint256 _reactionDeadline) returns()
func (_Modicum *ModicumTransactorSession) SetReactionDeadline(_reactionDeadline *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetReactionDeadline(&_Modicum.TransactOpts, _reactionDeadline)
}

// SetResourceProviderDepositMultiple is a paid mutator transaction binding the contract method 0xc785bd99.
//
// Solidity: function setResourceProviderDepositMultiple(uint256 multiple) returns()
func (_Modicum *ModicumTransactor) SetResourceProviderDepositMultiple(opts *bind.TransactOpts, multiple *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "setResourceProviderDepositMultiple", multiple)
}

// SetResourceProviderDepositMultiple is a paid mutator transaction binding the contract method 0xc785bd99.
//
// Solidity: function setResourceProviderDepositMultiple(uint256 multiple) returns()
func (_Modicum *ModicumSession) SetResourceProviderDepositMultiple(multiple *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetResourceProviderDepositMultiple(&_Modicum.TransactOpts, multiple)
}

// SetResourceProviderDepositMultiple is a paid mutator transaction binding the contract method 0xc785bd99.
//
// Solidity: function setResourceProviderDepositMultiple(uint256 multiple) returns()
func (_Modicum *ModicumTransactorSession) SetResourceProviderDepositMultiple(multiple *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.SetResourceProviderDepositMultiple(&_Modicum.TransactOpts, multiple)
}

// Test is a paid mutator transaction binding the contract method 0x29e99f07.
//
// Solidity: function test(uint256 value) returns()
func (_Modicum *ModicumTransactor) Test(opts *bind.TransactOpts, value *big.Int) (*types.Transaction, error) {
	return _Modicum.contract.Transact(opts, "test", value)
}

// Test is a paid mutator transaction binding the contract method 0x29e99f07.
//
// Solidity: function test(uint256 value) returns()
func (_Modicum *ModicumSession) Test(value *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.Test(&_Modicum.TransactOpts, value)
}

// Test is a paid mutator transaction binding the contract method 0x29e99f07.
//
// Solidity: function test(uint256 value) returns()
func (_Modicum *ModicumTransactorSession) Test(value *big.Int) (*types.Transaction, error) {
	return _Modicum.Contract.Test(&_Modicum.TransactOpts, value)
}

// ModicumDebugIterator is returned from FilterDebug and is used to iterate over the raw logs and unpacked data for Debug events raised by the Modicum contract.
type ModicumDebugIterator struct {
	Event *ModicumDebug // Event containing the contract specifics and raw log

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
func (it *ModicumDebugIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumDebug)
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
		it.Event = new(ModicumDebug)
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
func (it *ModicumDebugIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumDebugIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumDebug represents a Debug event raised by the Modicum contract.
type ModicumDebug struct {
	Value uint64
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterDebug is a free log retrieval operation binding the contract event 0x117173c3f483315f99860ad0cd3da109b9e36f175776c989bee6807ddc7554dd.
//
// Solidity: event Debug(uint64 value)
func (_Modicum *ModicumFilterer) FilterDebug(opts *bind.FilterOpts) (*ModicumDebugIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "Debug")
	if err != nil {
		return nil, err
	}
	return &ModicumDebugIterator{contract: _Modicum.contract, event: "Debug", logs: logs, sub: sub}, nil
}

// WatchDebug is a free log subscription operation binding the contract event 0x117173c3f483315f99860ad0cd3da109b9e36f175776c989bee6807ddc7554dd.
//
// Solidity: event Debug(uint64 value)
func (_Modicum *ModicumFilterer) WatchDebug(opts *bind.WatchOpts, sink chan<- *ModicumDebug) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "Debug")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumDebug)
				if err := _Modicum.contract.UnpackLog(event, "Debug", log); err != nil {
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

// ParseDebug is a log parse operation binding the contract event 0x117173c3f483315f99860ad0cd3da109b9e36f175776c989bee6807ddc7554dd.
//
// Solidity: event Debug(uint64 value)
func (_Modicum *ModicumFilterer) ParseDebug(log types.Log) (*ModicumDebug, error) {
	event := new(ModicumDebug)
	if err := _Modicum.contract.UnpackLog(event, "Debug", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumDebugArchIterator is returned from FilterDebugArch and is used to iterate over the raw logs and unpacked data for DebugArch events raised by the Modicum contract.
type ModicumDebugArchIterator struct {
	Event *ModicumDebugArch // Event containing the contract specifics and raw log

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
func (it *ModicumDebugArchIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumDebugArch)
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
		it.Event = new(ModicumDebugArch)
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
func (it *ModicumDebugArchIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumDebugArchIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumDebugArch represents a DebugArch event raised by the Modicum contract.
type ModicumDebugArch struct {
	Arch uint8
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterDebugArch is a free log retrieval operation binding the contract event 0xc06e1a5e77fb0f268e7a9d333c44019d6d8bb187b960fa3730205da5fa9a242f.
//
// Solidity: event DebugArch(uint8 arch)
func (_Modicum *ModicumFilterer) FilterDebugArch(opts *bind.FilterOpts) (*ModicumDebugArchIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "DebugArch")
	if err != nil {
		return nil, err
	}
	return &ModicumDebugArchIterator{contract: _Modicum.contract, event: "DebugArch", logs: logs, sub: sub}, nil
}

// WatchDebugArch is a free log subscription operation binding the contract event 0xc06e1a5e77fb0f268e7a9d333c44019d6d8bb187b960fa3730205da5fa9a242f.
//
// Solidity: event DebugArch(uint8 arch)
func (_Modicum *ModicumFilterer) WatchDebugArch(opts *bind.WatchOpts, sink chan<- *ModicumDebugArch) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "DebugArch")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumDebugArch)
				if err := _Modicum.contract.UnpackLog(event, "DebugArch", log); err != nil {
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

// ParseDebugArch is a log parse operation binding the contract event 0xc06e1a5e77fb0f268e7a9d333c44019d6d8bb187b960fa3730205da5fa9a242f.
//
// Solidity: event DebugArch(uint8 arch)
func (_Modicum *ModicumFilterer) ParseDebugArch(log types.Log) (*ModicumDebugArch, error) {
	event := new(ModicumDebugArch)
	if err := _Modicum.contract.UnpackLog(event, "DebugArch", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumDebugStringIterator is returned from FilterDebugString and is used to iterate over the raw logs and unpacked data for DebugString events raised by the Modicum contract.
type ModicumDebugStringIterator struct {
	Event *ModicumDebugString // Event containing the contract specifics and raw log

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
func (it *ModicumDebugStringIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumDebugString)
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
		it.Event = new(ModicumDebugString)
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
func (it *ModicumDebugStringIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumDebugStringIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumDebugString represents a DebugString event raised by the Modicum contract.
type ModicumDebugString struct {
	Str string
	Raw types.Log // Blockchain specific contextual infos
}

// FilterDebugString is a free log retrieval operation binding the contract event 0x20670ef4ff6910e98e7c43650896cc0feeab168b4ca974d0748d31c42706a1e9.
//
// Solidity: event DebugString(string str)
func (_Modicum *ModicumFilterer) FilterDebugString(opts *bind.FilterOpts) (*ModicumDebugStringIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "DebugString")
	if err != nil {
		return nil, err
	}
	return &ModicumDebugStringIterator{contract: _Modicum.contract, event: "DebugString", logs: logs, sub: sub}, nil
}

// WatchDebugString is a free log subscription operation binding the contract event 0x20670ef4ff6910e98e7c43650896cc0feeab168b4ca974d0748d31c42706a1e9.
//
// Solidity: event DebugString(string str)
func (_Modicum *ModicumFilterer) WatchDebugString(opts *bind.WatchOpts, sink chan<- *ModicumDebugString) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "DebugString")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumDebugString)
				if err := _Modicum.contract.UnpackLog(event, "DebugString", log); err != nil {
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

// ParseDebugString is a log parse operation binding the contract event 0x20670ef4ff6910e98e7c43650896cc0feeab168b4ca974d0748d31c42706a1e9.
//
// Solidity: event DebugString(string str)
func (_Modicum *ModicumFilterer) ParseDebugString(log types.Log) (*ModicumDebugString, error) {
	event := new(ModicumDebugString)
	if err := _Modicum.contract.UnpackLog(event, "DebugString", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumDebugUintIterator is returned from FilterDebugUint and is used to iterate over the raw logs and unpacked data for DebugUint events raised by the Modicum contract.
type ModicumDebugUintIterator struct {
	Event *ModicumDebugUint // Event containing the contract specifics and raw log

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
func (it *ModicumDebugUintIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumDebugUint)
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
		it.Event = new(ModicumDebugUint)
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
func (it *ModicumDebugUintIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumDebugUintIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumDebugUint represents a DebugUint event raised by the Modicum contract.
type ModicumDebugUint struct {
	Value *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterDebugUint is a free log retrieval operation binding the contract event 0xf0ed029e274dabb7636aeed7333cf47bc8c97dd6eb6d8faea6e9bfbd6620bebe.
//
// Solidity: event DebugUint(uint256 value)
func (_Modicum *ModicumFilterer) FilterDebugUint(opts *bind.FilterOpts) (*ModicumDebugUintIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "DebugUint")
	if err != nil {
		return nil, err
	}
	return &ModicumDebugUintIterator{contract: _Modicum.contract, event: "DebugUint", logs: logs, sub: sub}, nil
}

// WatchDebugUint is a free log subscription operation binding the contract event 0xf0ed029e274dabb7636aeed7333cf47bc8c97dd6eb6d8faea6e9bfbd6620bebe.
//
// Solidity: event DebugUint(uint256 value)
func (_Modicum *ModicumFilterer) WatchDebugUint(opts *bind.WatchOpts, sink chan<- *ModicumDebugUint) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "DebugUint")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumDebugUint)
				if err := _Modicum.contract.UnpackLog(event, "DebugUint", log); err != nil {
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

// ParseDebugUint is a log parse operation binding the contract event 0xf0ed029e274dabb7636aeed7333cf47bc8c97dd6eb6d8faea6e9bfbd6620bebe.
//
// Solidity: event DebugUint(uint256 value)
func (_Modicum *ModicumFilterer) ParseDebugUint(log types.Log) (*ModicumDebugUint, error) {
	event := new(ModicumDebugUint)
	if err := _Modicum.contract.UnpackLog(event, "DebugUint", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumEtherTransferredIterator is returned from FilterEtherTransferred and is used to iterate over the raw logs and unpacked data for EtherTransferred events raised by the Modicum contract.
type ModicumEtherTransferredIterator struct {
	Event *ModicumEtherTransferred // Event containing the contract specifics and raw log

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
func (it *ModicumEtherTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumEtherTransferred)
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
		it.Event = new(ModicumEtherTransferred)
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
func (it *ModicumEtherTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumEtherTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumEtherTransferred represents a EtherTransferred event raised by the Modicum contract.
type ModicumEtherTransferred struct {
	From  common.Address
	To    common.Address
	Value *big.Int
	Cause uint8
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterEtherTransferred is a free log retrieval operation binding the contract event 0x7e69a0d958a5252709f48635ae2d7514bc34ffc5f64e5b2aa70336d3e8bfcdf2.
//
// Solidity: event EtherTransferred(address _from, address to, uint256 value, uint8 cause)
func (_Modicum *ModicumFilterer) FilterEtherTransferred(opts *bind.FilterOpts) (*ModicumEtherTransferredIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "EtherTransferred")
	if err != nil {
		return nil, err
	}
	return &ModicumEtherTransferredIterator{contract: _Modicum.contract, event: "EtherTransferred", logs: logs, sub: sub}, nil
}

// WatchEtherTransferred is a free log subscription operation binding the contract event 0x7e69a0d958a5252709f48635ae2d7514bc34ffc5f64e5b2aa70336d3e8bfcdf2.
//
// Solidity: event EtherTransferred(address _from, address to, uint256 value, uint8 cause)
func (_Modicum *ModicumFilterer) WatchEtherTransferred(opts *bind.WatchOpts, sink chan<- *ModicumEtherTransferred) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "EtherTransferred")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumEtherTransferred)
				if err := _Modicum.contract.UnpackLog(event, "EtherTransferred", log); err != nil {
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

// ParseEtherTransferred is a log parse operation binding the contract event 0x7e69a0d958a5252709f48635ae2d7514bc34ffc5f64e5b2aa70336d3e8bfcdf2.
//
// Solidity: event EtherTransferred(address _from, address to, uint256 value, uint8 cause)
func (_Modicum *ModicumFilterer) ParseEtherTransferred(log types.Log) (*ModicumEtherTransferred, error) {
	event := new(ModicumEtherTransferred)
	if err := _Modicum.contract.UnpackLog(event, "EtherTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobAssignedForMediationIterator is returned from FilterJobAssignedForMediation and is used to iterate over the raw logs and unpacked data for JobAssignedForMediation events raised by the Modicum contract.
type ModicumJobAssignedForMediationIterator struct {
	Event *ModicumJobAssignedForMediation // Event containing the contract specifics and raw log

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
func (it *ModicumJobAssignedForMediationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobAssignedForMediation)
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
		it.Event = new(ModicumJobAssignedForMediation)
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
func (it *ModicumJobAssignedForMediationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobAssignedForMediationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobAssignedForMediation represents a JobAssignedForMediation event raised by the Modicum contract.
type ModicumJobAssignedForMediation struct {
	Addr    common.Address
	MatchId *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterJobAssignedForMediation is a free log retrieval operation binding the contract event 0x6e377930a82feda820d86166d6ed1dcbb8cb48305b1d20048584586cb1d424af.
//
// Solidity: event JobAssignedForMediation(address addr, uint256 matchId)
func (_Modicum *ModicumFilterer) FilterJobAssignedForMediation(opts *bind.FilterOpts) (*ModicumJobAssignedForMediationIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobAssignedForMediation")
	if err != nil {
		return nil, err
	}
	return &ModicumJobAssignedForMediationIterator{contract: _Modicum.contract, event: "JobAssignedForMediation", logs: logs, sub: sub}, nil
}

// WatchJobAssignedForMediation is a free log subscription operation binding the contract event 0x6e377930a82feda820d86166d6ed1dcbb8cb48305b1d20048584586cb1d424af.
//
// Solidity: event JobAssignedForMediation(address addr, uint256 matchId)
func (_Modicum *ModicumFilterer) WatchJobAssignedForMediation(opts *bind.WatchOpts, sink chan<- *ModicumJobAssignedForMediation) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobAssignedForMediation")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobAssignedForMediation)
				if err := _Modicum.contract.UnpackLog(event, "JobAssignedForMediation", log); err != nil {
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

// ParseJobAssignedForMediation is a log parse operation binding the contract event 0x6e377930a82feda820d86166d6ed1dcbb8cb48305b1d20048584586cb1d424af.
//
// Solidity: event JobAssignedForMediation(address addr, uint256 matchId)
func (_Modicum *ModicumFilterer) ParseJobAssignedForMediation(log types.Log) (*ModicumJobAssignedForMediation, error) {
	event := new(ModicumJobAssignedForMediation)
	if err := _Modicum.contract.UnpackLog(event, "JobAssignedForMediation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobCreatorAddedTrustedMediatorIterator is returned from FilterJobCreatorAddedTrustedMediator and is used to iterate over the raw logs and unpacked data for JobCreatorAddedTrustedMediator events raised by the Modicum contract.
type ModicumJobCreatorAddedTrustedMediatorIterator struct {
	Event *ModicumJobCreatorAddedTrustedMediator // Event containing the contract specifics and raw log

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
func (it *ModicumJobCreatorAddedTrustedMediatorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobCreatorAddedTrustedMediator)
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
		it.Event = new(ModicumJobCreatorAddedTrustedMediator)
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
func (it *ModicumJobCreatorAddedTrustedMediatorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobCreatorAddedTrustedMediatorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobCreatorAddedTrustedMediator represents a JobCreatorAddedTrustedMediator event raised by the Modicum contract.
type ModicumJobCreatorAddedTrustedMediator struct {
	Addr     common.Address
	Mediator common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterJobCreatorAddedTrustedMediator is a free log retrieval operation binding the contract event 0x14a79e8259e0076c5c8c81b13bbe7e12e248150c95f1ff3d9e0f2b377f8a15f2.
//
// Solidity: event JobCreatorAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) FilterJobCreatorAddedTrustedMediator(opts *bind.FilterOpts) (*ModicumJobCreatorAddedTrustedMediatorIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobCreatorAddedTrustedMediator")
	if err != nil {
		return nil, err
	}
	return &ModicumJobCreatorAddedTrustedMediatorIterator{contract: _Modicum.contract, event: "JobCreatorAddedTrustedMediator", logs: logs, sub: sub}, nil
}

// WatchJobCreatorAddedTrustedMediator is a free log subscription operation binding the contract event 0x14a79e8259e0076c5c8c81b13bbe7e12e248150c95f1ff3d9e0f2b377f8a15f2.
//
// Solidity: event JobCreatorAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) WatchJobCreatorAddedTrustedMediator(opts *bind.WatchOpts, sink chan<- *ModicumJobCreatorAddedTrustedMediator) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobCreatorAddedTrustedMediator")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobCreatorAddedTrustedMediator)
				if err := _Modicum.contract.UnpackLog(event, "JobCreatorAddedTrustedMediator", log); err != nil {
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

// ParseJobCreatorAddedTrustedMediator is a log parse operation binding the contract event 0x14a79e8259e0076c5c8c81b13bbe7e12e248150c95f1ff3d9e0f2b377f8a15f2.
//
// Solidity: event JobCreatorAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) ParseJobCreatorAddedTrustedMediator(log types.Log) (*ModicumJobCreatorAddedTrustedMediator, error) {
	event := new(ModicumJobCreatorAddedTrustedMediator)
	if err := _Modicum.contract.UnpackLog(event, "JobCreatorAddedTrustedMediator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobCreatorRegisteredIterator is returned from FilterJobCreatorRegistered and is used to iterate over the raw logs and unpacked data for JobCreatorRegistered events raised by the Modicum contract.
type ModicumJobCreatorRegisteredIterator struct {
	Event *ModicumJobCreatorRegistered // Event containing the contract specifics and raw log

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
func (it *ModicumJobCreatorRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobCreatorRegistered)
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
		it.Event = new(ModicumJobCreatorRegistered)
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
func (it *ModicumJobCreatorRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobCreatorRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobCreatorRegistered represents a JobCreatorRegistered event raised by the Modicum contract.
type ModicumJobCreatorRegistered struct {
	Addr        common.Address
	PenaltyRate *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterJobCreatorRegistered is a free log retrieval operation binding the contract event 0x4599183a5415a1a36544283668387752f2b7459b061d4c8434642289d6146a9e.
//
// Solidity: event JobCreatorRegistered(address addr, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) FilterJobCreatorRegistered(opts *bind.FilterOpts) (*ModicumJobCreatorRegisteredIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobCreatorRegistered")
	if err != nil {
		return nil, err
	}
	return &ModicumJobCreatorRegisteredIterator{contract: _Modicum.contract, event: "JobCreatorRegistered", logs: logs, sub: sub}, nil
}

// WatchJobCreatorRegistered is a free log subscription operation binding the contract event 0x4599183a5415a1a36544283668387752f2b7459b061d4c8434642289d6146a9e.
//
// Solidity: event JobCreatorRegistered(address addr, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) WatchJobCreatorRegistered(opts *bind.WatchOpts, sink chan<- *ModicumJobCreatorRegistered) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobCreatorRegistered")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobCreatorRegistered)
				if err := _Modicum.contract.UnpackLog(event, "JobCreatorRegistered", log); err != nil {
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

// ParseJobCreatorRegistered is a log parse operation binding the contract event 0x4599183a5415a1a36544283668387752f2b7459b061d4c8434642289d6146a9e.
//
// Solidity: event JobCreatorRegistered(address addr, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) ParseJobCreatorRegistered(log types.Log) (*ModicumJobCreatorRegistered, error) {
	event := new(ModicumJobCreatorRegistered)
	if err := _Modicum.contract.UnpackLog(event, "JobCreatorRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobOfferCanceledIterator is returned from FilterJobOfferCanceled and is used to iterate over the raw logs and unpacked data for JobOfferCanceled events raised by the Modicum contract.
type ModicumJobOfferCanceledIterator struct {
	Event *ModicumJobOfferCanceled // Event containing the contract specifics and raw log

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
func (it *ModicumJobOfferCanceledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobOfferCanceled)
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
		it.Event = new(ModicumJobOfferCanceled)
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
func (it *ModicumJobOfferCanceledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobOfferCanceledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobOfferCanceled represents a JobOfferCanceled event raised by the Modicum contract.
type ModicumJobOfferCanceled struct {
	OfferId *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterJobOfferCanceled is a free log retrieval operation binding the contract event 0x9e67088fa5663596c8d76e745a455b3c500bd75d1bdf673c41be74d362aaeb0b.
//
// Solidity: event JobOfferCanceled(uint256 offerId)
func (_Modicum *ModicumFilterer) FilterJobOfferCanceled(opts *bind.FilterOpts) (*ModicumJobOfferCanceledIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobOfferCanceled")
	if err != nil {
		return nil, err
	}
	return &ModicumJobOfferCanceledIterator{contract: _Modicum.contract, event: "JobOfferCanceled", logs: logs, sub: sub}, nil
}

// WatchJobOfferCanceled is a free log subscription operation binding the contract event 0x9e67088fa5663596c8d76e745a455b3c500bd75d1bdf673c41be74d362aaeb0b.
//
// Solidity: event JobOfferCanceled(uint256 offerId)
func (_Modicum *ModicumFilterer) WatchJobOfferCanceled(opts *bind.WatchOpts, sink chan<- *ModicumJobOfferCanceled) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobOfferCanceled")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobOfferCanceled)
				if err := _Modicum.contract.UnpackLog(event, "JobOfferCanceled", log); err != nil {
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

// ParseJobOfferCanceled is a log parse operation binding the contract event 0x9e67088fa5663596c8d76e745a455b3c500bd75d1bdf673c41be74d362aaeb0b.
//
// Solidity: event JobOfferCanceled(uint256 offerId)
func (_Modicum *ModicumFilterer) ParseJobOfferCanceled(log types.Log) (*ModicumJobOfferCanceled, error) {
	event := new(ModicumJobOfferCanceled)
	if err := _Modicum.contract.UnpackLog(event, "JobOfferCanceled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobOfferPostedPartOneIterator is returned from FilterJobOfferPostedPartOne and is used to iterate over the raw logs and unpacked data for JobOfferPostedPartOne events raised by the Modicum contract.
type ModicumJobOfferPostedPartOneIterator struct {
	Event *ModicumJobOfferPostedPartOne // Event containing the contract specifics and raw log

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
func (it *ModicumJobOfferPostedPartOneIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobOfferPostedPartOne)
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
		it.Event = new(ModicumJobOfferPostedPartOne)
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
func (it *ModicumJobOfferPostedPartOneIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobOfferPostedPartOneIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobOfferPostedPartOne represents a JobOfferPostedPartOne event raised by the Modicum contract.
type ModicumJobOfferPostedPartOne struct {
	OfferId             *big.Int
	Ijoid               *big.Int
	Addr                common.Address
	InstructionLimit    *big.Int
	BandwidthLimit      *big.Int
	InstructionMaxPrice *big.Int
	BandwidthMaxPrice   *big.Int
	CompletionDeadline  *big.Int
	Deposit             *big.Int
	MatchIncentive      *big.Int
	Raw                 types.Log // Blockchain specific contextual infos
}

// FilterJobOfferPostedPartOne is a free log retrieval operation binding the contract event 0x9306115efe7bad07bc21e212f2404b33e7376f164de102ec5e1e1b2938328db7.
//
// Solidity: event JobOfferPostedPartOne(uint256 offerId, uint256 ijoid, address addr, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 deposit, uint256 matchIncentive)
func (_Modicum *ModicumFilterer) FilterJobOfferPostedPartOne(opts *bind.FilterOpts) (*ModicumJobOfferPostedPartOneIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobOfferPostedPartOne")
	if err != nil {
		return nil, err
	}
	return &ModicumJobOfferPostedPartOneIterator{contract: _Modicum.contract, event: "JobOfferPostedPartOne", logs: logs, sub: sub}, nil
}

// WatchJobOfferPostedPartOne is a free log subscription operation binding the contract event 0x9306115efe7bad07bc21e212f2404b33e7376f164de102ec5e1e1b2938328db7.
//
// Solidity: event JobOfferPostedPartOne(uint256 offerId, uint256 ijoid, address addr, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 deposit, uint256 matchIncentive)
func (_Modicum *ModicumFilterer) WatchJobOfferPostedPartOne(opts *bind.WatchOpts, sink chan<- *ModicumJobOfferPostedPartOne) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobOfferPostedPartOne")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobOfferPostedPartOne)
				if err := _Modicum.contract.UnpackLog(event, "JobOfferPostedPartOne", log); err != nil {
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

// ParseJobOfferPostedPartOne is a log parse operation binding the contract event 0x9306115efe7bad07bc21e212f2404b33e7376f164de102ec5e1e1b2938328db7.
//
// Solidity: event JobOfferPostedPartOne(uint256 offerId, uint256 ijoid, address addr, uint256 instructionLimit, uint256 bandwidthLimit, uint256 instructionMaxPrice, uint256 bandwidthMaxPrice, uint256 completionDeadline, uint256 deposit, uint256 matchIncentive)
func (_Modicum *ModicumFilterer) ParseJobOfferPostedPartOne(log types.Log) (*ModicumJobOfferPostedPartOne, error) {
	event := new(ModicumJobOfferPostedPartOne)
	if err := _Modicum.contract.UnpackLog(event, "JobOfferPostedPartOne", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumJobOfferPostedPartTwoIterator is returned from FilterJobOfferPostedPartTwo and is used to iterate over the raw logs and unpacked data for JobOfferPostedPartTwo events raised by the Modicum contract.
type ModicumJobOfferPostedPartTwoIterator struct {
	Event *ModicumJobOfferPostedPartTwo // Event containing the contract specifics and raw log

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
func (it *ModicumJobOfferPostedPartTwoIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumJobOfferPostedPartTwo)
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
		it.Event = new(ModicumJobOfferPostedPartTwo)
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
func (it *ModicumJobOfferPostedPartTwoIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumJobOfferPostedPartTwoIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumJobOfferPostedPartTwo represents a JobOfferPostedPartTwo event raised by the Modicum contract.
type ModicumJobOfferPostedPartTwo struct {
	OfferId           *big.Int
	Addr              common.Address
	Hash              *big.Int
	Uri               string
	Directory         common.Address
	Arch              uint8
	RamLimit          *big.Int
	LocalStorageLimit *big.Int
	Extras            string
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterJobOfferPostedPartTwo is a free log retrieval operation binding the contract event 0xb08439994d9143544b8ae8aa0550882b0d882037947cab0dfa67fdfbe68b79ed.
//
// Solidity: event JobOfferPostedPartTwo(uint256 offerId, address addr, uint256 hash, string uri, address directory, uint8 arch, uint256 ramLimit, uint256 localStorageLimit, string extras)
func (_Modicum *ModicumFilterer) FilterJobOfferPostedPartTwo(opts *bind.FilterOpts) (*ModicumJobOfferPostedPartTwoIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "JobOfferPostedPartTwo")
	if err != nil {
		return nil, err
	}
	return &ModicumJobOfferPostedPartTwoIterator{contract: _Modicum.contract, event: "JobOfferPostedPartTwo", logs: logs, sub: sub}, nil
}

// WatchJobOfferPostedPartTwo is a free log subscription operation binding the contract event 0xb08439994d9143544b8ae8aa0550882b0d882037947cab0dfa67fdfbe68b79ed.
//
// Solidity: event JobOfferPostedPartTwo(uint256 offerId, address addr, uint256 hash, string uri, address directory, uint8 arch, uint256 ramLimit, uint256 localStorageLimit, string extras)
func (_Modicum *ModicumFilterer) WatchJobOfferPostedPartTwo(opts *bind.WatchOpts, sink chan<- *ModicumJobOfferPostedPartTwo) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "JobOfferPostedPartTwo")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumJobOfferPostedPartTwo)
				if err := _Modicum.contract.UnpackLog(event, "JobOfferPostedPartTwo", log); err != nil {
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

// ParseJobOfferPostedPartTwo is a log parse operation binding the contract event 0xb08439994d9143544b8ae8aa0550882b0d882037947cab0dfa67fdfbe68b79ed.
//
// Solidity: event JobOfferPostedPartTwo(uint256 offerId, address addr, uint256 hash, string uri, address directory, uint8 arch, uint256 ramLimit, uint256 localStorageLimit, string extras)
func (_Modicum *ModicumFilterer) ParseJobOfferPostedPartTwo(log types.Log) (*ModicumJobOfferPostedPartTwo, error) {
	event := new(ModicumJobOfferPostedPartTwo)
	if err := _Modicum.contract.UnpackLog(event, "JobOfferPostedPartTwo", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMatchClosedIterator is returned from FilterMatchClosed and is used to iterate over the raw logs and unpacked data for MatchClosed events raised by the Modicum contract.
type ModicumMatchClosedIterator struct {
	Event *ModicumMatchClosed // Event containing the contract specifics and raw log

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
func (it *ModicumMatchClosedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMatchClosed)
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
		it.Event = new(ModicumMatchClosed)
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
func (it *ModicumMatchClosedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMatchClosedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMatchClosed represents a MatchClosed event raised by the Modicum contract.
type ModicumMatchClosed struct {
	MatchId *big.Int
	Cost    *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterMatchClosed is a free log retrieval operation binding the contract event 0x30139f0bfbfc8bbba69979f515a178952806079af30fba4bc2e2e03e997769fc.
//
// Solidity: event MatchClosed(uint256 matchId, uint256 cost)
func (_Modicum *ModicumFilterer) FilterMatchClosed(opts *bind.FilterOpts) (*ModicumMatchClosedIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "MatchClosed")
	if err != nil {
		return nil, err
	}
	return &ModicumMatchClosedIterator{contract: _Modicum.contract, event: "MatchClosed", logs: logs, sub: sub}, nil
}

// WatchMatchClosed is a free log subscription operation binding the contract event 0x30139f0bfbfc8bbba69979f515a178952806079af30fba4bc2e2e03e997769fc.
//
// Solidity: event MatchClosed(uint256 matchId, uint256 cost)
func (_Modicum *ModicumFilterer) WatchMatchClosed(opts *bind.WatchOpts, sink chan<- *ModicumMatchClosed) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "MatchClosed")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMatchClosed)
				if err := _Modicum.contract.UnpackLog(event, "MatchClosed", log); err != nil {
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

// ParseMatchClosed is a log parse operation binding the contract event 0x30139f0bfbfc8bbba69979f515a178952806079af30fba4bc2e2e03e997769fc.
//
// Solidity: event MatchClosed(uint256 matchId, uint256 cost)
func (_Modicum *ModicumFilterer) ParseMatchClosed(log types.Log) (*ModicumMatchClosed, error) {
	event := new(ModicumMatchClosed)
	if err := _Modicum.contract.UnpackLog(event, "MatchClosed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMatchedIterator is returned from FilterMatched and is used to iterate over the raw logs and unpacked data for Matched events raised by the Modicum contract.
type ModicumMatchedIterator struct {
	Event *ModicumMatched // Event containing the contract specifics and raw log

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
func (it *ModicumMatchedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMatched)
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
		it.Event = new(ModicumMatched)
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
func (it *ModicumMatchedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMatchedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMatched represents a Matched event raised by the Modicum contract.
type ModicumMatched struct {
	Addr            common.Address
	MatchId         *big.Int
	JobOfferId      *big.Int
	ResourceOfferId *big.Int
	Mediator        common.Address
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterMatched is a free log retrieval operation binding the contract event 0xb30a5088883496975973e21a454aafabbe1548041f5c17c2f2e535532f82fbed.
//
// Solidity: event Matched(address addr, uint256 matchId, uint256 jobOfferId, uint256 resourceOfferId, address mediator)
func (_Modicum *ModicumFilterer) FilterMatched(opts *bind.FilterOpts) (*ModicumMatchedIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "Matched")
	if err != nil {
		return nil, err
	}
	return &ModicumMatchedIterator{contract: _Modicum.contract, event: "Matched", logs: logs, sub: sub}, nil
}

// WatchMatched is a free log subscription operation binding the contract event 0xb30a5088883496975973e21a454aafabbe1548041f5c17c2f2e535532f82fbed.
//
// Solidity: event Matched(address addr, uint256 matchId, uint256 jobOfferId, uint256 resourceOfferId, address mediator)
func (_Modicum *ModicumFilterer) WatchMatched(opts *bind.WatchOpts, sink chan<- *ModicumMatched) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "Matched")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMatched)
				if err := _Modicum.contract.UnpackLog(event, "Matched", log); err != nil {
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

// ParseMatched is a log parse operation binding the contract event 0xb30a5088883496975973e21a454aafabbe1548041f5c17c2f2e535532f82fbed.
//
// Solidity: event Matched(address addr, uint256 matchId, uint256 jobOfferId, uint256 resourceOfferId, address mediator)
func (_Modicum *ModicumFilterer) ParseMatched(log types.Log) (*ModicumMatched, error) {
	event := new(ModicumMatched)
	if err := _Modicum.contract.UnpackLog(event, "Matched", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMediationResultPostedIterator is returned from FilterMediationResultPosted and is used to iterate over the raw logs and unpacked data for MediationResultPosted events raised by the Modicum contract.
type ModicumMediationResultPostedIterator struct {
	Event *ModicumMediationResultPosted // Event containing the contract specifics and raw log

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
func (it *ModicumMediationResultPostedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMediationResultPosted)
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
		it.Event = new(ModicumMediationResultPosted)
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
func (it *ModicumMediationResultPostedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMediationResultPostedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMediationResultPosted represents a MediationResultPosted event raised by the Modicum contract.
type ModicumMediationResultPosted struct {
	MatchId          *big.Int
	Addr             common.Address
	Result           *big.Int
	FaultyParty      uint8
	Verdict          uint8
	Status           uint8
	Uri              string
	Hash             string
	InstructionCount *big.Int
	MediationCost    *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterMediationResultPosted is a free log retrieval operation binding the contract event 0xe9c156d5f11284ab5e2b9cb4e765d73089f8574ffdf9aacd1d25d764f669f18e.
//
// Solidity: event MediationResultPosted(uint256 matchId, address addr, uint256 result, uint8 faultyParty, uint8 verdict, uint8 status, string uri, string hash, uint256 instructionCount, uint256 mediationCost)
func (_Modicum *ModicumFilterer) FilterMediationResultPosted(opts *bind.FilterOpts) (*ModicumMediationResultPostedIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "MediationResultPosted")
	if err != nil {
		return nil, err
	}
	return &ModicumMediationResultPostedIterator{contract: _Modicum.contract, event: "MediationResultPosted", logs: logs, sub: sub}, nil
}

// WatchMediationResultPosted is a free log subscription operation binding the contract event 0xe9c156d5f11284ab5e2b9cb4e765d73089f8574ffdf9aacd1d25d764f669f18e.
//
// Solidity: event MediationResultPosted(uint256 matchId, address addr, uint256 result, uint8 faultyParty, uint8 verdict, uint8 status, string uri, string hash, uint256 instructionCount, uint256 mediationCost)
func (_Modicum *ModicumFilterer) WatchMediationResultPosted(opts *bind.WatchOpts, sink chan<- *ModicumMediationResultPosted) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "MediationResultPosted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMediationResultPosted)
				if err := _Modicum.contract.UnpackLog(event, "MediationResultPosted", log); err != nil {
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

// ParseMediationResultPosted is a log parse operation binding the contract event 0xe9c156d5f11284ab5e2b9cb4e765d73089f8574ffdf9aacd1d25d764f669f18e.
//
// Solidity: event MediationResultPosted(uint256 matchId, address addr, uint256 result, uint8 faultyParty, uint8 verdict, uint8 status, string uri, string hash, uint256 instructionCount, uint256 mediationCost)
func (_Modicum *ModicumFilterer) ParseMediationResultPosted(log types.Log) (*ModicumMediationResultPosted, error) {
	event := new(ModicumMediationResultPosted)
	if err := _Modicum.contract.UnpackLog(event, "MediationResultPosted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMediatorAddedSupportedFirstLayerIterator is returned from FilterMediatorAddedSupportedFirstLayer and is used to iterate over the raw logs and unpacked data for MediatorAddedSupportedFirstLayer events raised by the Modicum contract.
type ModicumMediatorAddedSupportedFirstLayerIterator struct {
	Event *ModicumMediatorAddedSupportedFirstLayer // Event containing the contract specifics and raw log

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
func (it *ModicumMediatorAddedSupportedFirstLayerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMediatorAddedSupportedFirstLayer)
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
		it.Event = new(ModicumMediatorAddedSupportedFirstLayer)
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
func (it *ModicumMediatorAddedSupportedFirstLayerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMediatorAddedSupportedFirstLayerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMediatorAddedSupportedFirstLayer represents a MediatorAddedSupportedFirstLayer event raised by the Modicum contract.
type ModicumMediatorAddedSupportedFirstLayer struct {
	Addr           common.Address
	FirstLayerHash *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterMediatorAddedSupportedFirstLayer is a free log retrieval operation binding the contract event 0xa4afa925b503907dc7699e5423d82b67cb3cce8e083875f0b14ae131ba8852d8.
//
// Solidity: event MediatorAddedSupportedFirstLayer(address addr, uint256 firstLayerHash)
func (_Modicum *ModicumFilterer) FilterMediatorAddedSupportedFirstLayer(opts *bind.FilterOpts) (*ModicumMediatorAddedSupportedFirstLayerIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "MediatorAddedSupportedFirstLayer")
	if err != nil {
		return nil, err
	}
	return &ModicumMediatorAddedSupportedFirstLayerIterator{contract: _Modicum.contract, event: "MediatorAddedSupportedFirstLayer", logs: logs, sub: sub}, nil
}

// WatchMediatorAddedSupportedFirstLayer is a free log subscription operation binding the contract event 0xa4afa925b503907dc7699e5423d82b67cb3cce8e083875f0b14ae131ba8852d8.
//
// Solidity: event MediatorAddedSupportedFirstLayer(address addr, uint256 firstLayerHash)
func (_Modicum *ModicumFilterer) WatchMediatorAddedSupportedFirstLayer(opts *bind.WatchOpts, sink chan<- *ModicumMediatorAddedSupportedFirstLayer) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "MediatorAddedSupportedFirstLayer")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMediatorAddedSupportedFirstLayer)
				if err := _Modicum.contract.UnpackLog(event, "MediatorAddedSupportedFirstLayer", log); err != nil {
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

// ParseMediatorAddedSupportedFirstLayer is a log parse operation binding the contract event 0xa4afa925b503907dc7699e5423d82b67cb3cce8e083875f0b14ae131ba8852d8.
//
// Solidity: event MediatorAddedSupportedFirstLayer(address addr, uint256 firstLayerHash)
func (_Modicum *ModicumFilterer) ParseMediatorAddedSupportedFirstLayer(log types.Log) (*ModicumMediatorAddedSupportedFirstLayer, error) {
	event := new(ModicumMediatorAddedSupportedFirstLayer)
	if err := _Modicum.contract.UnpackLog(event, "MediatorAddedSupportedFirstLayer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMediatorAddedTrustedDirectoryIterator is returned from FilterMediatorAddedTrustedDirectory and is used to iterate over the raw logs and unpacked data for MediatorAddedTrustedDirectory events raised by the Modicum contract.
type ModicumMediatorAddedTrustedDirectoryIterator struct {
	Event *ModicumMediatorAddedTrustedDirectory // Event containing the contract specifics and raw log

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
func (it *ModicumMediatorAddedTrustedDirectoryIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMediatorAddedTrustedDirectory)
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
		it.Event = new(ModicumMediatorAddedTrustedDirectory)
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
func (it *ModicumMediatorAddedTrustedDirectoryIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMediatorAddedTrustedDirectoryIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMediatorAddedTrustedDirectory represents a MediatorAddedTrustedDirectory event raised by the Modicum contract.
type ModicumMediatorAddedTrustedDirectory struct {
	Addr      common.Address
	Directory common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMediatorAddedTrustedDirectory is a free log retrieval operation binding the contract event 0x67d4e0df369cb5dee1cb58b6d75040efd1ee99797e4aa4be77057de2830d1769.
//
// Solidity: event MediatorAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) FilterMediatorAddedTrustedDirectory(opts *bind.FilterOpts) (*ModicumMediatorAddedTrustedDirectoryIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "MediatorAddedTrustedDirectory")
	if err != nil {
		return nil, err
	}
	return &ModicumMediatorAddedTrustedDirectoryIterator{contract: _Modicum.contract, event: "MediatorAddedTrustedDirectory", logs: logs, sub: sub}, nil
}

// WatchMediatorAddedTrustedDirectory is a free log subscription operation binding the contract event 0x67d4e0df369cb5dee1cb58b6d75040efd1ee99797e4aa4be77057de2830d1769.
//
// Solidity: event MediatorAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) WatchMediatorAddedTrustedDirectory(opts *bind.WatchOpts, sink chan<- *ModicumMediatorAddedTrustedDirectory) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "MediatorAddedTrustedDirectory")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMediatorAddedTrustedDirectory)
				if err := _Modicum.contract.UnpackLog(event, "MediatorAddedTrustedDirectory", log); err != nil {
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

// ParseMediatorAddedTrustedDirectory is a log parse operation binding the contract event 0x67d4e0df369cb5dee1cb58b6d75040efd1ee99797e4aa4be77057de2830d1769.
//
// Solidity: event MediatorAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) ParseMediatorAddedTrustedDirectory(log types.Log) (*ModicumMediatorAddedTrustedDirectory, error) {
	event := new(ModicumMediatorAddedTrustedDirectory)
	if err := _Modicum.contract.UnpackLog(event, "MediatorAddedTrustedDirectory", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumMediatorRegisteredIterator is returned from FilterMediatorRegistered and is used to iterate over the raw logs and unpacked data for MediatorRegistered events raised by the Modicum contract.
type ModicumMediatorRegisteredIterator struct {
	Event *ModicumMediatorRegistered // Event containing the contract specifics and raw log

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
func (it *ModicumMediatorRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumMediatorRegistered)
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
		it.Event = new(ModicumMediatorRegistered)
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
func (it *ModicumMediatorRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumMediatorRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumMediatorRegistered represents a MediatorRegistered event raised by the Modicum contract.
type ModicumMediatorRegistered struct {
	Addr              common.Address
	Arch              uint8
	InstructionPrice  *big.Int
	BandwidthPrice    *big.Int
	AvailabilityValue *big.Int
	VerificationCount *big.Int
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterMediatorRegistered is a free log retrieval operation binding the contract event 0x30f100afd350d3923afbf22d61bb461b018387a905b01935a82fd31739f7ecc8.
//
// Solidity: event MediatorRegistered(address addr, uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumFilterer) FilterMediatorRegistered(opts *bind.FilterOpts) (*ModicumMediatorRegisteredIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "MediatorRegistered")
	if err != nil {
		return nil, err
	}
	return &ModicumMediatorRegisteredIterator{contract: _Modicum.contract, event: "MediatorRegistered", logs: logs, sub: sub}, nil
}

// WatchMediatorRegistered is a free log subscription operation binding the contract event 0x30f100afd350d3923afbf22d61bb461b018387a905b01935a82fd31739f7ecc8.
//
// Solidity: event MediatorRegistered(address addr, uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumFilterer) WatchMediatorRegistered(opts *bind.WatchOpts, sink chan<- *ModicumMediatorRegistered) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "MediatorRegistered")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumMediatorRegistered)
				if err := _Modicum.contract.UnpackLog(event, "MediatorRegistered", log); err != nil {
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

// ParseMediatorRegistered is a log parse operation binding the contract event 0x30f100afd350d3923afbf22d61bb461b018387a905b01935a82fd31739f7ecc8.
//
// Solidity: event MediatorRegistered(address addr, uint8 arch, uint256 instructionPrice, uint256 bandwidthPrice, uint256 availabilityValue, uint256 verificationCount)
func (_Modicum *ModicumFilterer) ParseMediatorRegistered(log types.Log) (*ModicumMediatorRegistered, error) {
	event := new(ModicumMediatorRegistered)
	if err := _Modicum.contract.UnpackLog(event, "MediatorRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceOfferCanceledIterator is returned from FilterResourceOfferCanceled and is used to iterate over the raw logs and unpacked data for ResourceOfferCanceled events raised by the Modicum contract.
type ModicumResourceOfferCanceledIterator struct {
	Event *ModicumResourceOfferCanceled // Event containing the contract specifics and raw log

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
func (it *ModicumResourceOfferCanceledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceOfferCanceled)
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
		it.Event = new(ModicumResourceOfferCanceled)
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
func (it *ModicumResourceOfferCanceledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceOfferCanceledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceOfferCanceled represents a ResourceOfferCanceled event raised by the Modicum contract.
type ModicumResourceOfferCanceled struct {
	ResOfferId *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterResourceOfferCanceled is a free log retrieval operation binding the contract event 0xe6979ef995dbef56a8afac32269330ca9887368bbc6f449d0ee4c4ff75333146.
//
// Solidity: event ResourceOfferCanceled(uint256 resOfferId)
func (_Modicum *ModicumFilterer) FilterResourceOfferCanceled(opts *bind.FilterOpts) (*ModicumResourceOfferCanceledIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceOfferCanceled")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceOfferCanceledIterator{contract: _Modicum.contract, event: "ResourceOfferCanceled", logs: logs, sub: sub}, nil
}

// WatchResourceOfferCanceled is a free log subscription operation binding the contract event 0xe6979ef995dbef56a8afac32269330ca9887368bbc6f449d0ee4c4ff75333146.
//
// Solidity: event ResourceOfferCanceled(uint256 resOfferId)
func (_Modicum *ModicumFilterer) WatchResourceOfferCanceled(opts *bind.WatchOpts, sink chan<- *ModicumResourceOfferCanceled) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceOfferCanceled")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceOfferCanceled)
				if err := _Modicum.contract.UnpackLog(event, "ResourceOfferCanceled", log); err != nil {
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

// ParseResourceOfferCanceled is a log parse operation binding the contract event 0xe6979ef995dbef56a8afac32269330ca9887368bbc6f449d0ee4c4ff75333146.
//
// Solidity: event ResourceOfferCanceled(uint256 resOfferId)
func (_Modicum *ModicumFilterer) ParseResourceOfferCanceled(log types.Log) (*ModicumResourceOfferCanceled, error) {
	event := new(ModicumResourceOfferCanceled)
	if err := _Modicum.contract.UnpackLog(event, "ResourceOfferCanceled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceOfferPostedIterator is returned from FilterResourceOfferPosted and is used to iterate over the raw logs and unpacked data for ResourceOfferPosted events raised by the Modicum contract.
type ModicumResourceOfferPostedIterator struct {
	Event *ModicumResourceOfferPosted // Event containing the contract specifics and raw log

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
func (it *ModicumResourceOfferPostedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceOfferPosted)
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
		it.Event = new(ModicumResourceOfferPosted)
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
func (it *ModicumResourceOfferPostedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceOfferPostedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceOfferPosted represents a ResourceOfferPosted event raised by the Modicum contract.
type ModicumResourceOfferPosted struct {
	OfferId          *big.Int
	Addr             common.Address
	InstructionPrice *big.Int
	InstructionCap   *big.Int
	MemoryCap        *big.Int
	LocalStorageCap  *big.Int
	BandwidthCap     *big.Int
	BandwidthPrice   *big.Int
	Deposit          *big.Int
	Iroid            *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterResourceOfferPosted is a free log retrieval operation binding the contract event 0x0dbc425a2857617d2e3336b85e820c082e6403f272bdfc7b2805119a0be44121.
//
// Solidity: event ResourceOfferPosted(uint256 offerId, address addr, uint256 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 deposit, uint256 iroid)
func (_Modicum *ModicumFilterer) FilterResourceOfferPosted(opts *bind.FilterOpts) (*ModicumResourceOfferPostedIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceOfferPosted")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceOfferPostedIterator{contract: _Modicum.contract, event: "ResourceOfferPosted", logs: logs, sub: sub}, nil
}

// WatchResourceOfferPosted is a free log subscription operation binding the contract event 0x0dbc425a2857617d2e3336b85e820c082e6403f272bdfc7b2805119a0be44121.
//
// Solidity: event ResourceOfferPosted(uint256 offerId, address addr, uint256 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 deposit, uint256 iroid)
func (_Modicum *ModicumFilterer) WatchResourceOfferPosted(opts *bind.WatchOpts, sink chan<- *ModicumResourceOfferPosted) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceOfferPosted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceOfferPosted)
				if err := _Modicum.contract.UnpackLog(event, "ResourceOfferPosted", log); err != nil {
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

// ParseResourceOfferPosted is a log parse operation binding the contract event 0x0dbc425a2857617d2e3336b85e820c082e6403f272bdfc7b2805119a0be44121.
//
// Solidity: event ResourceOfferPosted(uint256 offerId, address addr, uint256 instructionPrice, uint256 instructionCap, uint256 memoryCap, uint256 localStorageCap, uint256 bandwidthCap, uint256 bandwidthPrice, uint256 deposit, uint256 iroid)
func (_Modicum *ModicumFilterer) ParseResourceOfferPosted(log types.Log) (*ModicumResourceOfferPosted, error) {
	event := new(ModicumResourceOfferPosted)
	if err := _Modicum.contract.UnpackLog(event, "ResourceOfferPosted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceProviderAddedSupportedFirstLayerIterator is returned from FilterResourceProviderAddedSupportedFirstLayer and is used to iterate over the raw logs and unpacked data for ResourceProviderAddedSupportedFirstLayer events raised by the Modicum contract.
type ModicumResourceProviderAddedSupportedFirstLayerIterator struct {
	Event *ModicumResourceProviderAddedSupportedFirstLayer // Event containing the contract specifics and raw log

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
func (it *ModicumResourceProviderAddedSupportedFirstLayerIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceProviderAddedSupportedFirstLayer)
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
		it.Event = new(ModicumResourceProviderAddedSupportedFirstLayer)
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
func (it *ModicumResourceProviderAddedSupportedFirstLayerIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceProviderAddedSupportedFirstLayerIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceProviderAddedSupportedFirstLayer represents a ResourceProviderAddedSupportedFirstLayer event raised by the Modicum contract.
type ModicumResourceProviderAddedSupportedFirstLayer struct {
	Addr       common.Address
	FirstLayer *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterResourceProviderAddedSupportedFirstLayer is a free log retrieval operation binding the contract event 0x6addfaf8048be7677397f667c84acb7e875091575bf511939b167e804d13a8fe.
//
// Solidity: event ResourceProviderAddedSupportedFirstLayer(address addr, uint256 firstLayer)
func (_Modicum *ModicumFilterer) FilterResourceProviderAddedSupportedFirstLayer(opts *bind.FilterOpts) (*ModicumResourceProviderAddedSupportedFirstLayerIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceProviderAddedSupportedFirstLayer")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceProviderAddedSupportedFirstLayerIterator{contract: _Modicum.contract, event: "ResourceProviderAddedSupportedFirstLayer", logs: logs, sub: sub}, nil
}

// WatchResourceProviderAddedSupportedFirstLayer is a free log subscription operation binding the contract event 0x6addfaf8048be7677397f667c84acb7e875091575bf511939b167e804d13a8fe.
//
// Solidity: event ResourceProviderAddedSupportedFirstLayer(address addr, uint256 firstLayer)
func (_Modicum *ModicumFilterer) WatchResourceProviderAddedSupportedFirstLayer(opts *bind.WatchOpts, sink chan<- *ModicumResourceProviderAddedSupportedFirstLayer) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceProviderAddedSupportedFirstLayer")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceProviderAddedSupportedFirstLayer)
				if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedSupportedFirstLayer", log); err != nil {
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

// ParseResourceProviderAddedSupportedFirstLayer is a log parse operation binding the contract event 0x6addfaf8048be7677397f667c84acb7e875091575bf511939b167e804d13a8fe.
//
// Solidity: event ResourceProviderAddedSupportedFirstLayer(address addr, uint256 firstLayer)
func (_Modicum *ModicumFilterer) ParseResourceProviderAddedSupportedFirstLayer(log types.Log) (*ModicumResourceProviderAddedSupportedFirstLayer, error) {
	event := new(ModicumResourceProviderAddedSupportedFirstLayer)
	if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedSupportedFirstLayer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceProviderAddedTrustedDirectoryIterator is returned from FilterResourceProviderAddedTrustedDirectory and is used to iterate over the raw logs and unpacked data for ResourceProviderAddedTrustedDirectory events raised by the Modicum contract.
type ModicumResourceProviderAddedTrustedDirectoryIterator struct {
	Event *ModicumResourceProviderAddedTrustedDirectory // Event containing the contract specifics and raw log

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
func (it *ModicumResourceProviderAddedTrustedDirectoryIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceProviderAddedTrustedDirectory)
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
		it.Event = new(ModicumResourceProviderAddedTrustedDirectory)
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
func (it *ModicumResourceProviderAddedTrustedDirectoryIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceProviderAddedTrustedDirectoryIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceProviderAddedTrustedDirectory represents a ResourceProviderAddedTrustedDirectory event raised by the Modicum contract.
type ModicumResourceProviderAddedTrustedDirectory struct {
	Addr      common.Address
	Directory common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterResourceProviderAddedTrustedDirectory is a free log retrieval operation binding the contract event 0xc8dca6a50f889635c514a90f99165d542d96eb943731beabec830ec3110ace73.
//
// Solidity: event ResourceProviderAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) FilterResourceProviderAddedTrustedDirectory(opts *bind.FilterOpts) (*ModicumResourceProviderAddedTrustedDirectoryIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceProviderAddedTrustedDirectory")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceProviderAddedTrustedDirectoryIterator{contract: _Modicum.contract, event: "ResourceProviderAddedTrustedDirectory", logs: logs, sub: sub}, nil
}

// WatchResourceProviderAddedTrustedDirectory is a free log subscription operation binding the contract event 0xc8dca6a50f889635c514a90f99165d542d96eb943731beabec830ec3110ace73.
//
// Solidity: event ResourceProviderAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) WatchResourceProviderAddedTrustedDirectory(opts *bind.WatchOpts, sink chan<- *ModicumResourceProviderAddedTrustedDirectory) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceProviderAddedTrustedDirectory")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceProviderAddedTrustedDirectory)
				if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedTrustedDirectory", log); err != nil {
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

// ParseResourceProviderAddedTrustedDirectory is a log parse operation binding the contract event 0xc8dca6a50f889635c514a90f99165d542d96eb943731beabec830ec3110ace73.
//
// Solidity: event ResourceProviderAddedTrustedDirectory(address addr, address directory)
func (_Modicum *ModicumFilterer) ParseResourceProviderAddedTrustedDirectory(log types.Log) (*ModicumResourceProviderAddedTrustedDirectory, error) {
	event := new(ModicumResourceProviderAddedTrustedDirectory)
	if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedTrustedDirectory", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceProviderAddedTrustedMediatorIterator is returned from FilterResourceProviderAddedTrustedMediator and is used to iterate over the raw logs and unpacked data for ResourceProviderAddedTrustedMediator events raised by the Modicum contract.
type ModicumResourceProviderAddedTrustedMediatorIterator struct {
	Event *ModicumResourceProviderAddedTrustedMediator // Event containing the contract specifics and raw log

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
func (it *ModicumResourceProviderAddedTrustedMediatorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceProviderAddedTrustedMediator)
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
		it.Event = new(ModicumResourceProviderAddedTrustedMediator)
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
func (it *ModicumResourceProviderAddedTrustedMediatorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceProviderAddedTrustedMediatorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceProviderAddedTrustedMediator represents a ResourceProviderAddedTrustedMediator event raised by the Modicum contract.
type ModicumResourceProviderAddedTrustedMediator struct {
	Addr     common.Address
	Mediator common.Address
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterResourceProviderAddedTrustedMediator is a free log retrieval operation binding the contract event 0xc46f5b1885edcb90bd9c37370760e20924bd0b4815bc3aa0999135f9e7447ba3.
//
// Solidity: event ResourceProviderAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) FilterResourceProviderAddedTrustedMediator(opts *bind.FilterOpts) (*ModicumResourceProviderAddedTrustedMediatorIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceProviderAddedTrustedMediator")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceProviderAddedTrustedMediatorIterator{contract: _Modicum.contract, event: "ResourceProviderAddedTrustedMediator", logs: logs, sub: sub}, nil
}

// WatchResourceProviderAddedTrustedMediator is a free log subscription operation binding the contract event 0xc46f5b1885edcb90bd9c37370760e20924bd0b4815bc3aa0999135f9e7447ba3.
//
// Solidity: event ResourceProviderAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) WatchResourceProviderAddedTrustedMediator(opts *bind.WatchOpts, sink chan<- *ModicumResourceProviderAddedTrustedMediator) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceProviderAddedTrustedMediator")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceProviderAddedTrustedMediator)
				if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedTrustedMediator", log); err != nil {
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

// ParseResourceProviderAddedTrustedMediator is a log parse operation binding the contract event 0xc46f5b1885edcb90bd9c37370760e20924bd0b4815bc3aa0999135f9e7447ba3.
//
// Solidity: event ResourceProviderAddedTrustedMediator(address addr, address mediator)
func (_Modicum *ModicumFilterer) ParseResourceProviderAddedTrustedMediator(log types.Log) (*ModicumResourceProviderAddedTrustedMediator, error) {
	event := new(ModicumResourceProviderAddedTrustedMediator)
	if err := _Modicum.contract.UnpackLog(event, "ResourceProviderAddedTrustedMediator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResourceProviderRegisteredIterator is returned from FilterResourceProviderRegistered and is used to iterate over the raw logs and unpacked data for ResourceProviderRegistered events raised by the Modicum contract.
type ModicumResourceProviderRegisteredIterator struct {
	Event *ModicumResourceProviderRegistered // Event containing the contract specifics and raw log

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
func (it *ModicumResourceProviderRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResourceProviderRegistered)
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
		it.Event = new(ModicumResourceProviderRegistered)
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
func (it *ModicumResourceProviderRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResourceProviderRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResourceProviderRegistered represents a ResourceProviderRegistered event raised by the Modicum contract.
type ModicumResourceProviderRegistered struct {
	Addr               common.Address
	Arch               uint8
	TimePerInstruction *big.Int
	PenaltyRate        *big.Int
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterResourceProviderRegistered is a free log retrieval operation binding the contract event 0xc1fa38743bb572e5e75169e44082ca0108a86cbce71a37e7590b45ace4ec80c9.
//
// Solidity: event ResourceProviderRegistered(address addr, uint8 arch, uint256 timePerInstruction, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) FilterResourceProviderRegistered(opts *bind.FilterOpts) (*ModicumResourceProviderRegisteredIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResourceProviderRegistered")
	if err != nil {
		return nil, err
	}
	return &ModicumResourceProviderRegisteredIterator{contract: _Modicum.contract, event: "ResourceProviderRegistered", logs: logs, sub: sub}, nil
}

// WatchResourceProviderRegistered is a free log subscription operation binding the contract event 0xc1fa38743bb572e5e75169e44082ca0108a86cbce71a37e7590b45ace4ec80c9.
//
// Solidity: event ResourceProviderRegistered(address addr, uint8 arch, uint256 timePerInstruction, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) WatchResourceProviderRegistered(opts *bind.WatchOpts, sink chan<- *ModicumResourceProviderRegistered) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResourceProviderRegistered")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResourceProviderRegistered)
				if err := _Modicum.contract.UnpackLog(event, "ResourceProviderRegistered", log); err != nil {
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

// ParseResourceProviderRegistered is a log parse operation binding the contract event 0xc1fa38743bb572e5e75169e44082ca0108a86cbce71a37e7590b45ace4ec80c9.
//
// Solidity: event ResourceProviderRegistered(address addr, uint8 arch, uint256 timePerInstruction, uint256 penaltyRate)
func (_Modicum *ModicumFilterer) ParseResourceProviderRegistered(log types.Log) (*ModicumResourceProviderRegistered, error) {
	event := new(ModicumResourceProviderRegistered)
	if err := _Modicum.contract.UnpackLog(event, "ResourceProviderRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResultPostedIterator is returned from FilterResultPosted and is used to iterate over the raw logs and unpacked data for ResultPosted events raised by the Modicum contract.
type ModicumResultPostedIterator struct {
	Event *ModicumResultPosted // Event containing the contract specifics and raw log

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
func (it *ModicumResultPostedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResultPosted)
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
		it.Event = new(ModicumResultPosted)
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
func (it *ModicumResultPostedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResultPostedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResultPosted represents a ResultPosted event raised by the Modicum contract.
type ModicumResultPosted struct {
	Addr             common.Address
	ResultId         *big.Int
	MatchId          *big.Int
	Status           uint8
	Uri              string
	Hash             string
	InstructionCount *big.Int
	BandwidthUsage   *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterResultPosted is a free log retrieval operation binding the contract event 0xc958f4b8e689bace1e5d4bc03cb4502a1547957e2ae45051374239cfee0f886b.
//
// Solidity: event ResultPosted(address addr, uint256 resultId, uint256 matchId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage)
func (_Modicum *ModicumFilterer) FilterResultPosted(opts *bind.FilterOpts) (*ModicumResultPostedIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResultPosted")
	if err != nil {
		return nil, err
	}
	return &ModicumResultPostedIterator{contract: _Modicum.contract, event: "ResultPosted", logs: logs, sub: sub}, nil
}

// WatchResultPosted is a free log subscription operation binding the contract event 0xc958f4b8e689bace1e5d4bc03cb4502a1547957e2ae45051374239cfee0f886b.
//
// Solidity: event ResultPosted(address addr, uint256 resultId, uint256 matchId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage)
func (_Modicum *ModicumFilterer) WatchResultPosted(opts *bind.WatchOpts, sink chan<- *ModicumResultPosted) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResultPosted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResultPosted)
				if err := _Modicum.contract.UnpackLog(event, "ResultPosted", log); err != nil {
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

// ParseResultPosted is a log parse operation binding the contract event 0xc958f4b8e689bace1e5d4bc03cb4502a1547957e2ae45051374239cfee0f886b.
//
// Solidity: event ResultPosted(address addr, uint256 resultId, uint256 matchId, uint8 status, string uri, string hash, uint256 instructionCount, uint256 bandwidthUsage)
func (_Modicum *ModicumFilterer) ParseResultPosted(log types.Log) (*ModicumResultPosted, error) {
	event := new(ModicumResultPosted)
	if err := _Modicum.contract.UnpackLog(event, "ResultPosted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumResultReactionIterator is returned from FilterResultReaction and is used to iterate over the raw logs and unpacked data for ResultReaction events raised by the Modicum contract.
type ModicumResultReactionIterator struct {
	Event *ModicumResultReaction // Event containing the contract specifics and raw log

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
func (it *ModicumResultReactionIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumResultReaction)
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
		it.Event = new(ModicumResultReaction)
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
func (it *ModicumResultReactionIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumResultReactionIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumResultReaction represents a ResultReaction event raised by the Modicum contract.
type ModicumResultReaction struct {
	Addr           common.Address
	ResultId       *big.Int
	MatchId        *big.Int
	ResultReaction *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterResultReaction is a free log retrieval operation binding the contract event 0x9cf99d589438cb7c5e0d92e45b92b87bcf5a2201479e72e8f31396be4a7505ab.
//
// Solidity: event ResultReaction(address addr, uint256 resultId, uint256 matchId, uint256 ResultReaction)
func (_Modicum *ModicumFilterer) FilterResultReaction(opts *bind.FilterOpts) (*ModicumResultReactionIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "ResultReaction")
	if err != nil {
		return nil, err
	}
	return &ModicumResultReactionIterator{contract: _Modicum.contract, event: "ResultReaction", logs: logs, sub: sub}, nil
}

// WatchResultReaction is a free log subscription operation binding the contract event 0x9cf99d589438cb7c5e0d92e45b92b87bcf5a2201479e72e8f31396be4a7505ab.
//
// Solidity: event ResultReaction(address addr, uint256 resultId, uint256 matchId, uint256 ResultReaction)
func (_Modicum *ModicumFilterer) WatchResultReaction(opts *bind.WatchOpts, sink chan<- *ModicumResultReaction) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "ResultReaction")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumResultReaction)
				if err := _Modicum.contract.UnpackLog(event, "ResultReaction", log); err != nil {
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

// ParseResultReaction is a log parse operation binding the contract event 0x9cf99d589438cb7c5e0d92e45b92b87bcf5a2201479e72e8f31396be4a7505ab.
//
// Solidity: event ResultReaction(address addr, uint256 resultId, uint256 matchId, uint256 ResultReaction)
func (_Modicum *ModicumFilterer) ParseResultReaction(log types.Log) (*ModicumResultReaction, error) {
	event := new(ModicumResultReaction)
	if err := _Modicum.contract.UnpackLog(event, "ResultReaction", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumPenaltyRateSetIterator is returned from FilterPenaltyRateSet and is used to iterate over the raw logs and unpacked data for PenaltyRateSet events raised by the Modicum contract.
type ModicumPenaltyRateSetIterator struct {
	Event *ModicumPenaltyRateSet // Event containing the contract specifics and raw log

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
func (it *ModicumPenaltyRateSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumPenaltyRateSet)
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
		it.Event = new(ModicumPenaltyRateSet)
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
func (it *ModicumPenaltyRateSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumPenaltyRateSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumPenaltyRateSet represents a PenaltyRateSet event raised by the Modicum contract.
type ModicumPenaltyRateSet struct {
	PenaltyRate *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterPenaltyRateSet is a free log retrieval operation binding the contract event 0xc3e81bef45eef27d5efd5d8de49b1398d66e449912912cf50fffdffd283e19e0.
//
// Solidity: event penaltyRateSet(uint256 penaltyRate)
func (_Modicum *ModicumFilterer) FilterPenaltyRateSet(opts *bind.FilterOpts) (*ModicumPenaltyRateSetIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "penaltyRateSet")
	if err != nil {
		return nil, err
	}
	return &ModicumPenaltyRateSetIterator{contract: _Modicum.contract, event: "penaltyRateSet", logs: logs, sub: sub}, nil
}

// WatchPenaltyRateSet is a free log subscription operation binding the contract event 0xc3e81bef45eef27d5efd5d8de49b1398d66e449912912cf50fffdffd283e19e0.
//
// Solidity: event penaltyRateSet(uint256 penaltyRate)
func (_Modicum *ModicumFilterer) WatchPenaltyRateSet(opts *bind.WatchOpts, sink chan<- *ModicumPenaltyRateSet) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "penaltyRateSet")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumPenaltyRateSet)
				if err := _Modicum.contract.UnpackLog(event, "penaltyRateSet", log); err != nil {
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

// ParsePenaltyRateSet is a log parse operation binding the contract event 0xc3e81bef45eef27d5efd5d8de49b1398d66e449912912cf50fffdffd283e19e0.
//
// Solidity: event penaltyRateSet(uint256 penaltyRate)
func (_Modicum *ModicumFilterer) ParsePenaltyRateSet(log types.Log) (*ModicumPenaltyRateSet, error) {
	event := new(ModicumPenaltyRateSet)
	if err := _Modicum.contract.UnpackLog(event, "penaltyRateSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ModicumReactionDeadlineSetIterator is returned from FilterReactionDeadlineSet and is used to iterate over the raw logs and unpacked data for ReactionDeadlineSet events raised by the Modicum contract.
type ModicumReactionDeadlineSetIterator struct {
	Event *ModicumReactionDeadlineSet // Event containing the contract specifics and raw log

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
func (it *ModicumReactionDeadlineSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ModicumReactionDeadlineSet)
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
		it.Event = new(ModicumReactionDeadlineSet)
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
func (it *ModicumReactionDeadlineSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ModicumReactionDeadlineSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ModicumReactionDeadlineSet represents a ReactionDeadlineSet event raised by the Modicum contract.
type ModicumReactionDeadlineSet struct {
	ReactionDeadline *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterReactionDeadlineSet is a free log retrieval operation binding the contract event 0xafb2b5bfefa5580825b0de810f205cc4f63195be2b7581e3a6d86853cda215a8.
//
// Solidity: event reactionDeadlineSet(uint256 reactionDeadline)
func (_Modicum *ModicumFilterer) FilterReactionDeadlineSet(opts *bind.FilterOpts) (*ModicumReactionDeadlineSetIterator, error) {

	logs, sub, err := _Modicum.contract.FilterLogs(opts, "reactionDeadlineSet")
	if err != nil {
		return nil, err
	}
	return &ModicumReactionDeadlineSetIterator{contract: _Modicum.contract, event: "reactionDeadlineSet", logs: logs, sub: sub}, nil
}

// WatchReactionDeadlineSet is a free log subscription operation binding the contract event 0xafb2b5bfefa5580825b0de810f205cc4f63195be2b7581e3a6d86853cda215a8.
//
// Solidity: event reactionDeadlineSet(uint256 reactionDeadline)
func (_Modicum *ModicumFilterer) WatchReactionDeadlineSet(opts *bind.WatchOpts, sink chan<- *ModicumReactionDeadlineSet) (event.Subscription, error) {

	logs, sub, err := _Modicum.contract.WatchLogs(opts, "reactionDeadlineSet")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ModicumReactionDeadlineSet)
				if err := _Modicum.contract.UnpackLog(event, "reactionDeadlineSet", log); err != nil {
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

// ParseReactionDeadlineSet is a log parse operation binding the contract event 0xafb2b5bfefa5580825b0de810f205cc4f63195be2b7581e3a6d86853cda215a8.
//
// Solidity: event reactionDeadlineSet(uint256 reactionDeadline)
func (_Modicum *ModicumFilterer) ParseReactionDeadlineSet(log types.Log) (*ModicumReactionDeadlineSet, error) {
	event := new(ModicumReactionDeadlineSet)
	if err := _Modicum.contract.UnpackLog(event, "reactionDeadlineSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
