package models

import (
	"strconv"
	"time"

	"github.com/tantralabs/tradeapi/iex"
)

// A comprehensive representation of an account across all markets of an exchange
type Account struct {
	AccountID    string
	ExchangeInfo ExchangeInfo
	BaseAsset    Asset
	Balances     map[string]*Asset
	Orders       map[string]*iex.Order
	MarketStates map[string]*MarketState
}

func NewAccount(exchange, baseSymbol string, exchangeInfo ExchangeInfo, balance float64) Account {
	accountID := exchange + "_" + strconv.Itoa(int(time.Now().UTC().UnixNano()/1000000))
	baseAsset := Asset{
		Symbol:   baseSymbol,
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
	baseMarketState := NewMarketStateFromExchange(baseSymbol, exchangeInfo, &account.BaseAsset.Quantity)
	account.MarketStates[baseSymbol] = &baseMarketState
	return account
}
