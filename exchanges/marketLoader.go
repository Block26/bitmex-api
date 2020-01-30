package exchanges

import (
	"errors"
	"log"

	"github.com/tantralabs/yantra/models"
)

func LoadMarket(exchange string, market string) (newMarket models.Market, err error) {
	if exchange == Bitmex {
		switch m := market; m {
		case "XBTUSD":
			return models.Market{
				Symbol:   "XBTUSD",
				Exchange: "bitmex",
				BaseAsset: models.Asset{
					Symbol:   "XBT",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "USD",
					Quantity: 0,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				TickSize:            1,
				QuantityTickSize:    1,
				QuantityPrecision:   0,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				Futures:             true,
				BulkCancelSupported: true,
			}, nil
		case "ETHUSD":
			return models.Market{
				Symbol:   "ETHUSD",
				Exchange: "bitmex",
				BaseAsset: models.Asset{
					Symbol:   "ETH",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "USD",
					Quantity: 0,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				TickSize:            1,
				QuantityTickSize:    1,
				QuantityPrecision:   0,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				Futures:             true,
				BulkCancelSupported: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == BitmexTestNet {
		switch m := market; m {
		case "XBTUSD":
			return models.Market{
				Symbol:      "XBTUSD",
				Exchange:    "bitmex",
				ExchangeURL: "https://testnet.bitmex.com",
				WSStream:    "testnet.bitmex.com",
				BaseAsset: models.Asset{
					Symbol:   "XBT",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "USD",
					Quantity: 0,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				TickSize:            1,
				QuantityTickSize:    1,
				QuantityPrecision:   0,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				Futures:             true,
				BulkCancelSupported: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == Binance {
		switch m := market; m {
		case "BTCUSDT":
			return models.Market{
				Symbol:      "BTCUSDT",
				Exchange:    "binance",
				ExchangeURL: "https://api.binance.us/",
				WSStream:    "stream.binance.us:9443",
				BaseAsset: models.Asset{
					Symbol:   "BTC",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "USDT",
					Quantity: 0,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    0.002,
				TickSize:            0.00000100,
				QuantityPrecision:   3,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				Futures:             false,
				BulkCancelSupported: false,
			}, nil
		case "BNBBTC":
			return models.Market{
				Symbol:      "BNBBTC",
				Exchange:    "binance",
				ExchangeURL: "https://api.binance.us/",
				WSStream:    "stream.binance.us:9443",
				BaseAsset: models.Asset{
					Symbol:   "BNB",
					Quantity: 100,
				},
				QuoteAsset: models.Asset{
					Symbol:   "BTC",
					Quantity: 1,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    0.1,
				TickSize:            0.0000001,
				QuantityPrecision:   2,
				PricePrecision:      7,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            0.00075,
				TakerFee:            0.00075,
				Futures:             false,
				BulkCancelSupported: false,
			}, nil
		case "ETHUSDT":
			return models.Market{
				Symbol:   "ETHUSDT",
				Exchange: "binance",
				BaseAsset: models.Asset{
					Symbol:   "ETH",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "USDT",
					Quantity: 0,
				},
				MaxLeverage:         1,
				MinimumOrderSize:    25,
				TickSize:            1,
				QuantityTickSize:    1,
				QuantityPrecision:   0,
				PricePrecision:      2,
				MaxOrders:           20,
				Slippage:            0.0,
				MakerFee:            -0.00025,
				TakerFee:            0.00075,
				Futures:             true,
				BulkCancelSupported: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == Deribit {
		switch m := market; m {
		case "BTC-PERPETUAL":
			return models.Market{
				Symbol:   "BTC-PERPETUAL",
				Exchange: "deribit",
				BaseAsset: models.Asset{
					Symbol:   "BTC",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "BTC-PERPETUAL",
					Quantity: 0,
				},
				MinimumOrderSize:        10,
				TickSize:                .5,
				QuantityTickSize:        10,
				QuantityPrecision:       0,
				PricePrecision:          2,
				MaxOrders:               20,
				Slippage:                0.0,
				MakerFee:                -0.00025,
				TakerFee:                0.00075,
				Futures:                 true,
				BulkCancelSupported:     false,
				Options:                 false,
				OptionContracts:         []models.OptionContract{},
				OptionStrikeInterval:    250.,
				OptionTickSize:          .1,
				OptionMinOrderSize:      .1,
				OptionMaxStrikePct:      300.,
				OptionMinStrikePct:      -50.,
				OptionMakerFee:          .0004,
				OptionTakerFee:          .0004,
				OptionSlippage:          .05,
				NumWeeklyOptions:        3,
				NumMonthlyOptions:       5,
				DenominatedInUnderlying: true,
			}, nil

		case "ETH-PERPETUAL":
			return models.Market{
				Symbol:   "ETH-PERPETUAL",
				Exchange: "deribit",
				BaseAsset: models.Asset{
					Symbol:   "ETH",
					Quantity: 5,
				},
				QuoteAsset: models.Asset{
					Symbol:   "ETH-PERPETUAL",
					Quantity: 0,
				},
				MinimumOrderSize:        10,
				TickSize:                .05,
				QuantityTickSize:        10,
				QuantityPrecision:       0,
				PricePrecision:          2,
				MaxOrders:               20,
				Slippage:                0.0,
				MakerFee:                -0.00025,
				TakerFee:                0.00075,
				Futures:                 true,
				BulkCancelSupported:     false,
				Options:                 false,
				OptionContracts:         []models.OptionContract{},
				OptionStrikeInterval:    10.,
				OptionTickSize:          .001,
				OptionMinOrderSize:      1.,
				OptionMaxStrikePct:      300.,
				OptionMinStrikePct:      -50.,
				OptionMakerFee:          .0004,
				OptionTakerFee:          .0004,
				OptionSlippage:          .05,
				NumWeeklyOptions:        3,
				NumMonthlyOptions:       5,
				DenominatedInUnderlying: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	} else if exchange == DeribitTestNet {
		switch m := market; m {
		case "BTC-PERPETUAL":
			return models.Market{
				Symbol:      "BTC-PERPETUAL",
				Exchange:    "deribit",
				ExchangeURL: "test.deribit.com",
				WSStream:    "test.deribit.com",
				BaseAsset: models.Asset{
					Symbol:   "BTC",
					Quantity: 1,
				},
				QuoteAsset: models.Asset{
					Symbol:   "BTC-PERPETUAL",
					Quantity: 0,
				},
				MinimumOrderSize:        10,
				TickSize:                .5,
				QuantityTickSize:        10,
				QuantityPrecision:       0,
				PricePrecision:          2,
				MaxOrders:               20,
				Slippage:                0.0,
				MakerFee:                -0.00025,
				TakerFee:                0.00075,
				Futures:                 true,
				BulkCancelSupported:     false,
				Options:                 false,
				OptionContracts:         []models.OptionContract{},
				OptionStrikeInterval:    250.,
				OptionTickSize:          .1,
				OptionMinOrderSize:      .1,
				OptionMaxStrikePct:      300.,
				OptionMinStrikePct:      -50.,
				OptionMakerFee:          .0004,
				OptionTakerFee:          .0004,
				OptionSlippage:          .05,
				NumWeeklyOptions:        3,
				NumMonthlyOptions:       5,
				DenominatedInUnderlying: true,
			}, nil
		default:
			log.Println(m, "is not supported for exchange", exchange)
		}
	}
	err = errors.New("error: exchange not supported")
	return
}
