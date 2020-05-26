package yantra

import (
	"log"
	"testing"
	"time"

	aop "github.com/tantralabs/auto-order-placement"
	"github.com/tantralabs/logger"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
)

const symbol = "BTCUSDT"
const exchange = "binance"
const di = 60

func TestBinanceFuturesOrders(t *testing.T) {
	/// Testing behavior of placing a market order for btc, then going back into USDT
	/// Binance does not do an inverse swap.
	/// All futures orders placed must be denominated in a btc amount.
	startingBalance := 100.
	exchangeInfo, _ := exchanges.LoadExchangeInfo(exchange)
	exchangeInfo.QuantityPrecision = 0.0000001
	exchangeInfo.Spot = false
	exchangeInfo.Futures = true
	exchangeInfo.DenominatedInQuote = true
	account := models.NewAccount(symbol, exchangeInfo, startingBalance)
	algo := models.Algo{
		Name:              "binance-orders-test",
		Account:           account,
		DataLength:        1,
		RebalanceInterval: "1m",
		LogBacktest:       false,
		LogLevel:          logger.LogLevel().Debug,
		BacktestLogLevel:  logger.LogLevel().Info,
	}

	algo.Account.MarketStates[symbol].Info.MarketType = 0 // Futures
	tradingEngine := NewTradingEngine(&algo, -1)
	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 01, 0, 03, 0, 0, time.UTC)
	// tradingEngine.SetupTest(start, end, Rebalance, SetupData)
	tradingEngine.RunTest(start, end, PlaceOrder, SetupData)

	expectedBTCPosition := 0.005
	expectedUSDTBalance := 64.12294999999999
	if algo.Account.MarketStates[symbol].Position != expectedBTCPosition || algo.Account.MarketStates[symbol].Balance != expectedUSDTBalance {
		if algo.Account.MarketStates[symbol].Position != expectedBTCPosition {
			t.Error("Position has changed from", expectedBTCPosition, "to", algo.Account.MarketStates[symbol].Position)
		} else {
			t.Error("Balance has changed from", expectedUSDTBalance, "to", algo.Account.MarketStates[symbol].Balance)
		}
	}
}
func TestBinanceFuturesAopOrders(t *testing.T) {
	/// Testing behavior of AOP
	/// Binance does not do an inverse swap.
	/// All futures orders placed must be denominated in a btc amount.
	startingBalance := 10000.
	exchangeInfo, _ := exchanges.LoadExchangeInfo(exchange)
	exchangeInfo.QuantityPrecision = 0.0000001
	exchangeInfo.Spot = false
	exchangeInfo.Futures = true
	exchangeInfo.DenominatedInQuote = true

	account := models.NewAccount(symbol, exchangeInfo, startingBalance)
	algo := models.Algo{
		Name:              "binance-aop-test",
		Account:           account,
		DataLength:        1,
		RebalanceInterval: "1m",
		LogBacktest:       false,
		LogLevel:          logger.LogLevel().Debug,
		BacktestLogLevel:  logger.LogLevel().Info,
	}

	aopParams := aop.Params{
		DataInterval:            di,
		BaseLeverage:            1, // The base leverage for the Algo, 1 would be 100%, 0.5 would be 50% of the MaxLeverage defined by Market.
		BaseEntryOrderSize:      0.20,
		BaseExitOrderSize:       1,
		BaseDeleverageOrderSize: 0.05,
	}
	aop.SetParameters(&algo, aopParams, symbol)

	algo.Account.MarketStates[symbol].Info.MarketType = 0 // Futures
	tradingEngine := NewTradingEngine(&algo, -1)
	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 01, 00, 15, 0, 0, time.UTC)
	// tradingEngine.SetupTest(start, end, Rebalance, SetupData)
	tradingEngine.RunTest(start, end, PlaceAopOrder, SetupData)

	expectedBTCPosition := 6.800676676384579e-06
	expectedUSDTBalance := 100.04884538418077
	if algo.Account.MarketStates[symbol].Position != expectedBTCPosition || algo.Account.MarketStates[symbol].Balance != expectedUSDTBalance {
		if algo.Account.MarketStates[symbol].Position != expectedBTCPosition {
			t.Error("Position has changed from", expectedBTCPosition, "to", algo.Account.MarketStates[symbol].Position)
		} else {
			t.Error("Balance has changed from", expectedUSDTBalance, "to", algo.Account.MarketStates[symbol].Balance)
		}
	}
}
func SetupData(algo *models.Algo) {
}

func Rebalance(algo *models.Algo) {
}

func PlaceOrder(algo *models.Algo) {
	if algo.Account.MarketStates[symbol].Position > 0.0001 {
		order := iex.Order{
			Market:   symbol,
			Currency: symbol,
			Amount:   .005,
			// Rate:     orderPrice,
			Type: "market",
			Side: "sell",
		}
		log.Println("--------New Trade--------")
		log.Printf("Placing order %v\n", order)
		algo.Client.PlaceOrder(order)
	} else {
		// orderPrice := 7000.
		order := iex.Order{
			Market:   symbol,
			Currency: symbol,
			Amount:   .01,
			// Rate:     orderPrice,
			Type: "market",
			Side: "buy",
		}
		log.Println("--------New Trade--------")
		log.Printf("Placing order %v\n", order)
		algo.Client.PlaceOrder(order)
	}

	// log.Println(algo.Account.MarketStates[symbol].Balance)
}

func PlaceAopOrder(algo *models.Algo) {
	aopParams := aop.GetParameters(algo, symbol)
	if algo.Account.MarketStates[symbol].Leverage > .02 {
		weight := 1
		leverageTarget := aopParams.BaseLeverage
		t := aop.Targets{
			Weight:              weight,
			Leverage:            leverageTarget,
			EntryOrderSize:      aopParams.BaseEntryOrderSize,
			DeleverageOrderSize: aopParams.BaseDeleverageOrderSize,
			ExitOrderSize:       aopParams.BaseExitOrderSize,
		}
		log.Println("--------New Trade--------")
		aop.PlaceOrders(algo, symbol, t)
	} else {
		weight := -1
		leverageTarget := aopParams.BaseLeverage
		t := aop.Targets{
			Weight:              weight,
			Leverage:            leverageTarget,
			EntryOrderSize:      aopParams.BaseEntryOrderSize,
			DeleverageOrderSize: aopParams.BaseDeleverageOrderSize,
			ExitOrderSize:       aopParams.BaseExitOrderSize,
		}
		log.Println("--------New Trade--------")
		aop.PlaceOrders(algo, symbol, t)
	}

	// log.Println(algo.Account.MarketStates[symbol].Balance)
}
