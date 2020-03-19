package models

import (
	"log"
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
	baseMarketState := NewMarketStateFromInfo(baseMarketInfo, &account.BaseAsset.Quantity)

	account.MarketStates[baseSymbol] = &baseMarketState
	return account
}
