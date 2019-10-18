package algo

import (
	"fmt"
	"log"

	exchangehandler "gitlab.com/raedah/tradeapi"
	"gitlab.com/raedah/tradeapi/iex"
)

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, *Algo)) {
	config = loadConfiguration(settingsFile, secret)
	// settings = loadConfiguration("dev/mm/testnet", true)
	log.Println(config)
	// fireDB := setupFirebase()

	exchangeVars := iex.ExchangeConf{
		Exchange:       "binance",
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	base_currency := "USD"
	quote_currency := "BTC"

	ex, err := exchangehandler.GetExchangeType(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}

	//Get base and quote balances
	baseCurrencyBalance, err := ex.GetBalance(base_currency)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("base_currency balance: %+v \n", baseCurrencyBalance)

	algo.Asset.BaseBalance = baseCurrencyBalance.Available

	quoteCurrencyBalance, err := ex.GetBalance(quote_currency)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("quote_currency balance: %+v \n", quoteCurrencyBalance)
	algo.Asset.Quantity = quoteCurrencyBalance.Available

	mkt, err := ex.GetMarketSummary(quote_currency + base_currency)
	fmt.Printf("markets: %+v \n", mkt)
	algo.Asset.Price = mkt.Last
	rebalance(mkt.Last, &algo)
	algo.BuyOrders.Quantity = mulArr(algo.BuyOrders.Quantity, (algo.Asset.Buying * mkt.Last))
	algo.SellOrders.Quantity = mulArr(algo.SellOrders.Quantity, (algo.Asset.Selling * mkt.Last))
	log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
	log.Println("Total Buy BTC", (algo.Asset.Buying))
	log.Println("Total Buy USD", (algo.Asset.Buying * mkt.Last))
	log.Println("Total Sell BTC", (algo.Asset.Selling))
	log.Println("Total Sell USD", (algo.Asset.Selling * mkt.Last))
	// log.Println("Local order length", len(orders))
	log.Println("New order length", len(algo.BuyOrders.Quantity), len(algo.SellOrders.Quantity))
	// log.Println("Buys", algo.BuyOrders.Quantity)
	// log.Println("Sells", algo.SellOrders.Quantity)
	// log.Println("New order length", len(algo.BuyOrders.Price), len(algo.SellOrders.Price))
	// b.PlaceOrdersOnBook(config.Symbol, algo.BuyOrders, algo.SellOrders, orders)
	algo.logState(mkt.Last)
	// updateAlgo(fireDB, "mm")
}
