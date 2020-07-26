package models

import (
	"errors"
	"log"
	"strings"
)

const (
	Bitmex         = "bitmex"
	BitmexTestNet  = "bitmex-test"
	Deribit        = "deribit"
	DeribitTestNet = "deribit-test"
	Binance        = "binance"
)

// Enum for a market's type (spot, future, or option)
type MarketType int

// How do we want the mock exchange to fill our order?
type FillType int

const (
	Future MarketType = iota
	Option
	Spot
)

var marketTypes = [...]string{
	"Future",
	"Option",
	"Spot",
}

const (
	Limit FillType = iota
	// Open
	Close
	Worst
	MeanOC
	MeanHL
)

var fillTypes = [...]string{
	"limit",
	// "open",
	"close",
	"worst",
	"meanOC",
	"meanHL",
}

type OptionType int

const (
	Call OptionType = iota
	Put
)

var OptionTypes = [...]string{
	"Call",
	"Put",
}

// Stateless market information.
type MarketInfo struct {
	Symbol                  string     // a string representing the entire market (i.e. XBTUSD)
	BaseSymbol              string     // string representing base asset (i.e. XBT)
	QuoteSymbol             string     // string representing quote asset (i.e. USD)
	MarketType              MarketType // spot, future, or option
	FillType                FillType   // how we want the exchange to fill our orders
	Exchange                string     // the exchange hosting this market
	ExchangeURL             string     // REST API endpoint
	WSStream                string     // Websocket API endpoint
	MaxOrders               int        // maximum number of outstanding orders for this market
	MakerFee                float64    // maker fee as decimal (i.e. -.00025 is .025% rebate)
	TakerFee                float64    // taker fee as decimal
	Slippage                float64    // expected slippage for this market, as decimal
	PricePrecision          float64    // minimum granularity of order price
	QuantityPrecision       float64    // minimum granularity of order amount
	MinimumOrderSize        float64    // absolute minimum size of order
	MaxLeverage             float64    // max leverage as ratio (i.e. 100x = 100.)
	BulkCancelSupported     bool       // can we cancel multiple orders in one REST API call?
	DenominatedInUnderlying bool       // is this market amount quoted in terms of base asset?
	DenominatedInQuote      bool

	// Only used for options
	Strike           float64
	Expiry           int
	OptionType       OptionType
	UnderlyingSymbol string
}

// Returns a new market info struct given a symbol and exchange information.
func NewMarketInfo(symbol string, exchangeInfo ExchangeInfo) MarketInfo {
	return MarketInfo{
		Symbol:                  symbol,
		Exchange:                exchangeInfo.Exchange,
		ExchangeURL:             exchangeInfo.ExchangeURL,
		WSStream:                exchangeInfo.WSStream,
		MaxOrders:               exchangeInfo.MaxOrders,
		MakerFee:                exchangeInfo.MakerFee,
		TakerFee:                exchangeInfo.TakerFee,
		Slippage:                exchangeInfo.Slippage,
		PricePrecision:          exchangeInfo.PricePrecision,
		QuantityPrecision:       exchangeInfo.QuantityPrecision,
		MinimumOrderSize:        exchangeInfo.MinimumOrderSize,
		MaxLeverage:             exchangeInfo.MaxLeverage,
		BulkCancelSupported:     exchangeInfo.BulkCancelSupported,
		DenominatedInUnderlying: exchangeInfo.DenominatedInUnderlying,
		DenominatedInQuote:      exchangeInfo.DenominatedInQuote,
	}
}

// Constructs new market info given an exchange and market symbol. Throw an error if the pair is not recognized.
func LoadMarketInfo(exchange string, market string) (newMarket MarketInfo, err error) {
	if exchange == Bitmex {
		switch m := market; m {
		case "XBTUSD":
			return MarketInfo{
				Symbol:              "XBTUSD",
				Exchange:            "bitmex",
				BaseSymbol:          "XBT",
				QuoteSymbol:         "USD",
				MarketType:          Future,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    1,
				QuantityPrecision:   1.,
				PricePrecision:      .5,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				BulkCancelSupported: true,
				DenominatedInQuote:  false,
			}, nil
		case "ETHUSD":
			return MarketInfo{
				Symbol:              "ETHUSD",
				Exchange:            "bitmex",
				BaseSymbol:          "ETH",
				QuoteSymbol:         "USD",
				MarketType:          Future,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   1.,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				BulkCancelSupported: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == BitmexTestNet {
		switch m := market; m {
		case "XBTUSD":
			return MarketInfo{
				Symbol:              "XBTUSD",
				Exchange:            "bitmex",
				BaseSymbol:          "XBT",
				QuoteSymbol:         "USD",
				MarketType:          Future,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   1.,
				PricePrecision:      .5,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				BulkCancelSupported: true,
			}, nil
		case "ETHUSD":
			return MarketInfo{
				Symbol:              "ETHUSD",
				Exchange:            "bitmex",
				BaseSymbol:          "ETH",
				QuoteSymbol:         "USD",
				MarketType:          Future,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   0.,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				BulkCancelSupported: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == Binance {
		switch m := market; m {
		case "BTCUSDT":
			return MarketInfo{
				Symbol:              "BTCUSDT",
				Exchange:            "binance",
				ExchangeURL:         "https://api.binance.us/",
				WSStream:            "stream.binance.us:9443",
				BaseSymbol:          "BTC",
				QuoteSymbol:         "USDT",
				MarketType:          Spot,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    0.002,
				QuantityPrecision:   0.0000001,
				PricePrecision:      0.00000100,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				BulkCancelSupported: false,
			}, nil
		case "BNBBTC":
			return MarketInfo{
				Symbol:              "BNBUSDT",
				Exchange:            "binance",
				ExchangeURL:         "https://api.binance.us/",
				WSStream:            "stream.binance.us:9443",
				BaseSymbol:          "BNB",
				QuoteSymbol:         "BTC",
				MarketType:          Spot,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    0.002,
				QuantityPrecision:   1,
				PricePrecision:      0.00000100,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				BulkCancelSupported: false,
			}, nil
		case "ETHUSDT":
			return MarketInfo{
				Symbol:              "ETHUSDT",
				Exchange:            "binance",
				ExchangeURL:         "https://api.binance.us/",
				WSStream:            "stream.binance.us:9443",
				BaseSymbol:          "ETH",
				QuoteSymbol:         "USDT",
				MarketType:          Spot,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    0.002,
				QuantityPrecision:   1,
				PricePrecision:      0.00000100,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				BulkCancelSupported: false,
			}, nil
		case "EOSUSDT":
			return MarketInfo{
				Symbol:              "EOSUSDT",
				Exchange:            "binance",
				ExchangeURL:         "https://api.binance.us/",
				WSStream:            "stream.binance.us:9443",
				BaseSymbol:          "EOS",
				QuoteSymbol:         "USDT",
				MarketType:          Spot,
				FillType:            Close,
				MaxLeverage:         1,
				MinimumOrderSize:    0.002,
				QuantityPrecision:   1,
				PricePrecision:      0.00000100,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				BulkCancelSupported: false,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == Deribit {
		if strings.Contains(market, "BTC") {
			return MarketInfo{
				Symbol:                  market,
				Exchange:                "deribit",
				BaseSymbol:              "BTC",
				QuoteSymbol:             "USD",
				MarketType:              Future,
				FillType:                Close,
				MinimumOrderSize:        10,
				QuantityPrecision:       10,
				PricePrecision:          .5,
				MaxOrders:               20,
				Slippage:                0.0,
				MakerFee:                -0.00025,
				TakerFee:                0.00075,
				BulkCancelSupported:     false,
				DenominatedInUnderlying: true,
			}, nil
		} else if strings.Contains(market, "ETH") {
			return MarketInfo{
				Symbol:                  market,
				Exchange:                "deribit",
				BaseSymbol:              "ETH",
				QuoteSymbol:             "USD",
				MarketType:              Future,
				FillType:                Close,
				MinimumOrderSize:        10,
				QuantityPrecision:       10,
				PricePrecision:          .5,
				MaxOrders:               20,
				Slippage:                0.0,
				MakerFee:                -0.00025,
				TakerFee:                0.00075,
				BulkCancelSupported:     false,
				DenominatedInUnderlying: true,
			}, nil
		}
		log.Println(market, "is not supported for exchange", exchange)
	} else if exchange == DeribitTestNet {
		switch m := market; m {
		case "BTC-PERPETUAL":
			return MarketInfo{
				Symbol:              "BTC-PERPETUAL",
				Exchange:            "deribit",
				ExchangeURL:         "test.deribit.com",
				WSStream:            "test.deribit.com",
				BaseSymbol:          "BTC",
				QuoteSymbol:         "USD",
				MarketType:          Future,
				FillType:            Close,
				MinimumOrderSize:    10,
				QuantityPrecision:   10,
				PricePrecision:      .5,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				BulkCancelSupported: false,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	}
	err = errors.New("error: exchange not supported")
	return
}
