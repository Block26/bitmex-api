package models

import (
	"log"
	"strconv"
	"time"

	"github.com/tantralabs/tradeapi/iex"
)

// A comprehensive representation of an account across all markets of an exchange
type Account struct {
	AccountID        string                  // A unique identifier for the account
	ExchangeInfo     ExchangeInfo            // Information relevant to this exchange
	BaseAsset        Asset                   // Contains information about the base asset for this account i.e. XBT on bitmex
	Balances         map[string]*Asset       // Maps asset symbols to their respective balances
	Orders           map[string]*iex.Order   // Maps market symbols to their respective orders
	MarketStates     map[string]*MarketState // Maps market symbols to their respective states
	UnrealizedProfit float64                 // Total unrealized profit
	RealizedProfit   float64                 // Total realized profit
	Profit           float64                 // Total profit
}

// Constructs new account struct given a base symbol, exchange info, and base balance amount
func NewAccount(baseSymbol string, exchangeInfo ExchangeInfo, balance float64) Account {
	accountID := exchangeInfo.Exchange + "_" + strconv.Itoa(int(time.Now().UTC().UnixNano()/1000000))
	baseMarketInfo, err := LoadMarketInfo(exchangeInfo.Exchange, baseSymbol)
	if err != nil {
		log.Fatal("Error loading market info")
	}
	baseAsset := Asset{
		Symbol:   baseMarketInfo.BaseSymbol,
		Quantity: balance,
	}
	account := Account{
		AccountID:    accountID,
		ExchangeInfo: exchangeInfo,
		BaseAsset:    baseAsset,
		Balances: map[string]*Asset{
			baseAsset.Symbol: &baseAsset,
		},
		Orders:       make(map[string]*iex.Order),
		MarketStates: make(map[string]*MarketState),
	}
	baseMarketState := NewMarketStateFromInfo(baseMarketInfo, account.BaseAsset.Quantity)

	account.MarketStates[baseSymbol] = &baseMarketState
	return account
}
