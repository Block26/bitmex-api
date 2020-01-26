package yantra

import (
	"log"
	"testing"
	"time"

	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
)

func setupAlgo() Algo {
	market, _ := exchanges.LoadMarket(exchanges.Bitmex, "XBTUSD")
	algo := Algo{
		Name:                "test",
		Market:              market,
		Params:              map[string]interface{}{},
		RebalanceInterval:   exchanges.RebalanceInterval().Minute,
		FillType:            exchanges.FillType().Close,
		EntryOrderSize:      0.2,
		ExitOrderSize:       0.2,
		DeleverageOrderSize: 0.05,
		DataLength:          901,
		LeverageTarget:      1,
		AutoOrderPlacement:  true,
	}

	algo.Market.Price = models.Bar{Open: 100, High: 100, Close: 100, Low: 100}
	return algo
}

func resetOrders(algo *Algo) {
	algo.Market.BuyOrders = models.OrderArray{make([]float64, 0), make([]float64, 0)}
	algo.Market.SellOrders = models.OrderArray{make([]float64, 0), make([]float64, 0)}
}

func setupExchange() iex.IExchange {
	exchangeVars := iex.ExchangeConf{
		Exchange:       "bitmex",
		AccountID:      "test",
		OutputResponse: false,
	}

	ex, _ := tradeapi.New(exchangeVars)
	return ex
}

func TestPositionUpdate(t *testing.T) {
	algo := setupAlgo()
	positions := []iex.WsPosition{
		iex.WsPosition{
			Symbol:       "XBTUSD",
			AvgCostPrice: 1000,
			CurrentQty:   100,
		},
	}
	algo.updatePositions(positions)
	if algo.Market.QuoteAsset.Quantity != 100 {
		t.Error("Quote Asset Balance is not updating properly")
	}
	if algo.Market.AverageCost != 1000 {
		t.Error("Average Cost is not updating properly")
	}
}

func TestBalanceUpdate(t *testing.T) {
	algo := setupAlgo()
	balances := []iex.WSBalance{
		iex.WSBalance{
			Asset:   "XBTUSD",
			Balance: 1,
		},
	}
	algo.updateAlgoBalances(balances)
	if algo.Market.BaseAsset.Quantity != 1 {
		t.Error("Base Asset Balance is not updating properly")
	}
}

func TestOrderUpdate(t *testing.T) {
	ex := setupExchange()
	orderStatus = ex.GetPotentialOrderStatus()
	var localOrders []iex.Order

	// Place an order
	newOrders := []iex.Order{
		iex.Order{
			OrderID:   "1",
			Symbol:    "XBTUSD",
			Amount:    100,
			Rate:      100,
			OrdStatus: "new",
		},
	}
	localOrders = updateLocalOrders(localOrders, newOrders)
	if len(localOrders) != 1 {
		t.Error("Orders not updating properly")
	}

	// Place another order
	newOrders = []iex.Order{
		iex.Order{
			OrderID:   "2",
			Symbol:    "XBTUSD",
			Amount:    100,
			Rate:      100,
			OrdStatus: "new",
		},
	}
	localOrders = updateLocalOrders(localOrders, newOrders)
	if len(localOrders) != 2 {
		t.Error("Orders not updating properly")
	}

	// Cancel An order
	newOrders = []iex.Order{
		iex.Order{
			OrderID:   "2",
			Symbol:    "XBTUSD",
			Amount:    100,
			Rate:      100,
			OrdStatus: "Canceled",
		},
	}
	localOrders = updateLocalOrders(localOrders, newOrders)
	if len(localOrders) != 1 {
		t.Error("Orders not updating properly")
	}
}

func TestSetupOrdersAutoOrderPlacement(t *testing.T) {
	algo := setupAlgo()
	price := 100.

	// WEIGHT 0 Tests
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 0
	algo.Market.Weight = 0
	algo.shouldHaveQuantity = 0

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 0 and Quantity is 0 you should not be placing orders")
	}

	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is 0 and Quantity is 0 you should not be placing orders")
	}

	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 10
	algo.Market.Weight = 0
	algo.shouldHaveQuantity = 0

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 0 and Quantity is 0 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 0 and Quantity is 10 you should be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 10 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 10 ")
	}

	// ensure second order is still 10
	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 0 and Quantity is 0 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 0 and Quantity is 1 you should be placing sell orders")
	}

	if algo.Market.SellOrders.Quantity[0] != 10 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 10")
	}

	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = -10
	algo.Market.Weight = 0
	algo.shouldHaveQuantity = -1

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 1 {
		t.Error("Weight is 0 and Quantity is -1 you should be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is 0 and Quantity is 1 you should not be placing sell orders")
	}

	// WEIGHT 1 TESTS
	// BUY FROM 0
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 0
	algo.Market.Weight = 1
	algo.shouldHaveQuantity = 0

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 0 you should be placing orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 0 you should not be placing orders")
	}

	// DONT BUY TOO MUCH
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 100
	algo.Market.Weight = 1
	algo.shouldHaveQuantity = 100

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 100 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 1 you should be placing sell orders")
	}

	// DELEVERAGE
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 110
	algo.Market.Weight = 1
	algo.shouldHaveQuantity = 110

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing sell orders TO DELEVERAGE")
	}
	if algo.Market.SellOrders.Quantity[0] != 5 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 5")
	}

	// WEIGHT -1 TESTS
	// SELL FROM 0
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 0
	algo.Market.Weight = -1
	algo.shouldHaveQuantity = 0

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is 0 you should not be placing orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is -1 and Quantity is 0 you should be placing orders")
	}

	// DONT SELL TOO MUCH
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = -100
	algo.Market.Weight = -1
	algo.shouldHaveQuantity = -100

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is 100 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is 1 you should be placing sell orders")
	}
}

func TestDeleverageShort(t *testing.T) {
	algo := setupAlgo()
	price := 100.
	// DELEVERAGE
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = -110
	algo.Market.Weight = -1
	algo.shouldHaveQuantity = -110

	algo.logState(time.Now())
	algo.setupOrders(price)

	log.Println(algo.Market.BuyOrders.Quantity)
	if len(algo.Market.BuyOrders.Quantity) != 1 {
		t.Error("Weight is -1 and Quantity is -110 you should be placing buy orders")
	}
	if algo.Market.BuyOrders.Quantity[0] != 5 {
		t.Error(algo.Market.BuyOrders.Quantity[0], "!= 5")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is -110 you should not be placing sell orders")
	}

	algo.logState(time.Now())
	algo.setupOrders(price)

	log.Println(algo.Market.BuyOrders.Quantity)
	if len(algo.Market.BuyOrders.Quantity) != 1 {
		t.Error("Weight is -1 and Quantity is -110 you should be placing buy orders")
	}
	if algo.Market.BuyOrders.Quantity[0] != 10 {
		t.Error(algo.Market.BuyOrders.Quantity[0], "!= 10")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is -110 you should not be placing sell orders")
	}

	algo.logState(time.Now())
	algo.setupOrders(price)

	log.Println(algo.Market.BuyOrders.Quantity)
	if len(algo.Market.BuyOrders.Quantity) != 1 {
		t.Error("Weight is -1 and Quantity is -110 you should be placing buy orders")
	}
	if algo.Market.BuyOrders.Quantity[0] != 10 {
		t.Error(algo.Market.BuyOrders.Quantity[0], "!= 10")
	}
	if len(algo.Market.SellOrders.Quantity) != 0 {
		t.Error("Weight is -1 and Quantity is -110 you should not be placing sell orders")
	}
}

func TestDeleverageLong(t *testing.T) {
	algo := setupAlgo()
	price := 100.
	// DELEVERAGE
	resetOrders(&algo)
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 110
	algo.Market.Weight = 1
	algo.shouldHaveQuantity = 110

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing buy orders")
	}

	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("You should be placing sell orders to deleverage")
	}

	if algo.Market.SellOrders.Quantity[0] != 5 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 5")
	}

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is -10 you should not be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 10 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 10")
	}

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 10 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 10")
	}

	resetOrders(&algo)
	algo.LeverageTarget = 1
	algo.Market.BaseAsset.Quantity = 1
	algo.Market.QuoteAsset.Quantity = 110
	algo.Market.Weight = 1
	algo.shouldHaveQuantity = 110

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 5 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 5")
	}

	algo.LeverageTarget = 0.5
	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 10 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 10")
	}

	algo.logState(time.Now())
	algo.setupOrders(price)

	if len(algo.Market.BuyOrders.Quantity) != 0 {
		t.Error("Weight is 1 and Quantity is 110 you should not be placing buy orders")
	}
	if len(algo.Market.SellOrders.Quantity) != 1 {
		t.Error("Weight is 1 and Quantity is 110 you should be placing sell orders")
	}
	if algo.Market.SellOrders.Quantity[0] != 15 {
		t.Error(algo.Market.SellOrders.Quantity[0], "!= 15")
	}

}
