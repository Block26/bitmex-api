package models

import (
	"errors"
	"log"
)

const (
	Bitmex         = "bitmex"
	BitmexTestNet  = "bitmex-test"
	Deribit        = "deribit"
	DeribitTestNet = "deribit-test"
	Binance        = "binance"
)

type MarketType int

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
	Symbol                  string
	BaseSymbol              string
	QuoteSymbol             string
	MarketType              MarketType
	Exchange                string
	ExchangeURL             string
	WSStream                string
	MaxOrders               int
	MakerFee                float64
	TakerFee                float64
	Slippage                float64
	PricePrecision          float64
	QuantityPrecision       float64
	MinimumOrderSize        float64
	MaxLeverage             float64
	BulkCancelSupported     bool
	DenominatedInUnderlying bool

	// Only used for options
	Strike           float64
	Expiry           int
	OptionType       OptionType
	UnderlyingSymbol string
}

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
	}
}

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
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   1,
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
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   0,
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
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   1,
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
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				QuantityPrecision:   0,
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
		case "BNBBTC":
			return MarketInfo{
				Symbol:              "BNBUSDT",
				Exchange:            "binance",
				ExchangeURL:         "https://api.binance.us/",
				WSStream:            "stream.binance.us:9443",
				BaseSymbol:          "BNB",
				QuoteSymbol:         "BTC",
				MarketType:          Spot,
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
		switch m := market; m {
		case "BTC-PERPETUAL":
			return MarketInfo{
				Symbol:                  "BTC-PERPETUAL",
				Exchange:                "deribit",
				BaseSymbol:              "BTC",
				QuoteSymbol:             "USD",
				MarketType:              Future,
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

		case "ETH-PERPETUAL":
			return MarketInfo{
				Symbol:                  "ETH-PERPETUAL",
				Exchange:                "deribit",
				BaseSymbol:              "ETH",
				QuoteSymbol:             "USD",
				MarketType:              Future,
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
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
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
