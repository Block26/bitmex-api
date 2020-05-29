package exchanges

import (
	"github.com/tantralabs/yantra/models"
)

// type ExchangeInfo struct {
// 	Exchange string

// 	// These are also configurable at the market level, but we can set defaults here for convenience
// 	ExchangeURL             string
// 	WSStream                string
// 	MaxOrders               int
// 	MakerFee                float64
// 	TakerFee                float64
// 	Slippage                float64
// 	PricePrecision          float64
// 	QuantityPrecision       float64
// 	MinimumOrderSize        float64
// 	MaxLeverage             float64
// 	BulkCancelSupported     bool
// 	DenominatedInUnderlying bool

// 	// Supported symbols for the exchange
// 	Symbols map[string]bool

// 	// Indicate whether each of these is supported
// 	Spot    bool
// 	Futures bool
// 	Options bool
// }

func LoadExchangeInfo(exchange string) (exchangeInfo models.ExchangeInfo, err error) {
	if exchange == Bitmex {
		symbols := map[string]bool{
			"XBTUSD": true,
			"ETHUSD": true,
		}
		return models.ExchangeInfo{
			Exchange:            exchange,
			MaxLeverage:         1,
			MinimumOrderSize:    25,
			QuantityPrecision:   1,
			PricePrecision:      .5,
			MaxOrders:           20,
			Slippage:            0.0,
			MakerFee:            -0.00025,
			TakerFee:            0.00075,
			BulkCancelSupported: true,
			Futures:             true,
			Symbols:             symbols,
		}, nil
	} else if exchange == BitmexTestNet {
		symbols := map[string]bool{
			"XBTUSD": true,
			"ETHUSD": true,
		}
		return models.ExchangeInfo{
			Exchange:            exchange,
			IsTestnet:           true,
			MaxLeverage:         1,
			MinimumOrderSize:    25,
			QuantityPrecision:   1,
			PricePrecision:      .5,
			MaxOrders:           20,
			Slippage:            0.0,
			MakerFee:            -0.00025,
			TakerFee:            0.00075,
			BulkCancelSupported: true,
			Futures:             true,
			Symbols:             symbols,
		}, nil
	} else if exchange == Binance {
		symbols := map[string]bool{
			"BTCUSDT": true,
			"ETHUSDT": true,
			"BNBBTC":  true,
			"EOSUSDT": true,
		}
		return models.ExchangeInfo{
			Exchange:            exchange,
			MaxLeverage:         1,
			MinimumOrderSize:    0.002,
			QuantityPrecision:   1.,
			PricePrecision:      0.00000100,
			MaxOrders:           20,
			Slippage:            0.0,
			MakerFee:            0.00075,
			TakerFee:            0.00075,
			BulkCancelSupported: false,
			Spot:                true,
			Symbols:             symbols,
		}, nil
	} else if exchange == Deribit {
		symbols := map[string]bool{
			"BTC-PERPETUAL": true,
			"ETH-PERPETUAL": true,
		}
		return models.ExchangeInfo{
			Exchange:                exchange,
			MinimumOrderSize:        10,
			QuantityPrecision:       10,
			PricePrecision:          .5,
			MaxOrders:               20,
			Slippage:                0.0,
			MakerFee:                -0.00025,
			TakerFee:                0.00075,
			BulkCancelSupported:     false,
			DenominatedInQuote:      false,
			DenominatedInUnderlying: true,
			Futures:                 true,
			Options:                 false,
			Symbols:                 symbols,
		}, nil
	} else if exchange == DeribitTestNet {
		symbols := map[string]bool{
			"BTC-PERPETUAL": true,
		}
		return models.ExchangeInfo{
			Exchange:                exchange,
			MinimumOrderSize:        10,
			QuantityPrecision:       10,
			PricePrecision:          .5,
			MaxOrders:               20,
			Slippage:                0.0,
			MakerFee:                -0.00025,
			TakerFee:                0.00075,
			BulkCancelSupported:     false,
			DenominatedInUnderlying: true,
			Futures:                 true,
			Options:                 true,
			Symbols:                 symbols,
		}, nil
	}
	return
}
