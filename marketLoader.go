package algo

import (
	"errors"
	"log"
)

func LoadMarket(exchange string, market string) (newMarket Market, err error) {

	if exchange == "bitmex" {
		switch m := market; m {
		case "XBTUSD":
			return Market{
				Symbol:           "XBTUSD",
				Exchange:         "bitmex",
				ExchangeURL:      "https://testnet.bitmex.com",
				WSStream:         "testnet.bitmex.com",
				BaseAsset:        "XBT",
				QuoteAsset:       "USD",
				MinimumOrderSize: 25,
				TickSize:         1,
				MaxOrders:        20,
				MakerFee:         -0.00025,
				TakerFee:         0.00075,
				Futures:          true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == "binance" {
		switch m := market; m {
		case "BTCUSDT":
			return Market{
				Symbol:           "BTCUSDT",
				Exchange:         "binance",
				ExchangeURL:      "https://api.binance.us/",
				WSStream:         "stream.binance.us:9443",
				BaseAsset:        "BTC",
				QuoteAsset:       "USDT",
				MinimumOrderSize: 0.002,
				TickSize:         1,
				MaxOrders:        20,
				MakerFee:         0.00075,
				TakerFee:         0.00075,
				Futures:          false,
			}, nil
		case "BNBBTC":
			return Market{
				Symbol:           "BNBBTC",
				Exchange:         "binance",
				ExchangeURL:      "https://api.binance.us/",
				WSStream:         "stream.binance.us:9443",
				BaseAsset:        "BNB",
				QuoteAsset:       "BTC",
				MinimumOrderSize: 0.002,
				TickSize:         0.00001,
				MaxOrders:        20,
				MakerFee:         0.00075,
				TakerFee:         0.00075,
				Futures:          false,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	}
	err = errors.New("error: exchange not supported")
	return
}
