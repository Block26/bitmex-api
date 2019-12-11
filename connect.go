package algo

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/tantralabs/TheAlgoV2/data"
	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
)

var orderStatus iex.OrderStatus
var firstTrade bool
var firstPositionUpdate bool
var shouldHaveQuantity float64

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, Algo) Algo, setupData func([]*models.Bar, Algo)) {
	firstTrade = true
	firstPositionUpdate = true
	config := loadConfiguration(settingsFile, secret)
	fmt.Printf("Loaded config for %v \n", algo.Market.Exchange)
	// We instantiate a new repository targeting the given path (the .git folder)
	// r, err := git.PlainOpen(".")
	// CheckIfError(err)
	// ... retrieving the HEAD reference
	// ref, err := r.Head()
	commitHash = "test" //ref.Hash().String()
	// CheckIfError(err)

	exchangeVars := iex.ExchangeConf{
		Exchange:       algo.Market.Exchange,
		ServerUrl:      algo.Market.ExchangeURL,
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	fmt.Printf("Connecting to %v with key %v and secret %v, id %v\n", exchangeVars.Exchange, exchangeVars.ApiKey, exchangeVars.ApiSecret, exchangeVars.AccountID)
	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("Getting potential order status...\n")
	orderStatus = ex.GetPotentialOrderStatus()
	fmt.Printf("Got potential order status %v\n", orderStatus)

	fmt.Printf("Getting data with symbol %v, decisioninterval %v, datalength %v\n", algo.Market.Symbol, algo.DecisionInterval, algo.DataLength+1)
	// localBars := make([]*models.Bar, 0)
	localBars := data.GetData("XBTUSD", algo.DecisionInterval, algo.DataLength+1)
	fmt.Printf("Got local bars: %v\n", len(localBars))
	// log.Println(len(localBars), "downloaded")

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := ex.GetBalances()
	algo.updateAlgoBalances(balances)

	// SUBSCRIBE TO WEBSOCKETS

	// channels to subscribe to
	symbol := strings.ToLower(algo.Market.Symbol)
	//Ordering is important, get wallet and position first then market info
	subscribeInfos := []iex.WSSubscribeInfo{
		{Name: iex.WS_WALLET, Symbol: symbol},
		{Name: iex.WS_ORDER, Symbol: symbol},
		{Name: iex.WS_POSITION, Symbol: symbol},
		{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol, Market: iex.WSMarketType{Contract: iex.WS_SWAP}},
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 2),
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.WSOrder, 2),
	}

	// Start the websocket.
	err = ex.StartWS(&iex.WsConfig{Host: algo.Market.WSStream, //"testnet.bitmex.com", //"stream.binance.us:9443",
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		fmt.Println(err)
	}

	var localOrders []iex.WSOrder

	for {
		select {
		case positions := <-channels.PositionChan:
			algo.updatePositions(positions)
		case trade := <-channels.TradeBinChan:
			algo.updateState(ex, trade[0], &localBars, setupData)
			algo = rebalance(trade[0].Close, algo)
			algo.setupOrders()
			algo.PlaceOrdersOnBook(ex, localOrders)
			algo.logState()
		case newOrders := <-channels.OrderChan:
			// log.Println("update channels.OrderChan")
			localOrders = UpdateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			algo.updateAlgoBalances(update.Balance)
		}
	}
}

func (algo *Algo) updatePositions(positions []iex.WsPosition) {
	log.Println("Position Update:", positions)
	position := positions[0]
	algo.Market.QuoteAsset.Quantity = float64(position.CurrentQty)
	if math.Abs(algo.Market.QuoteAsset.Quantity) > 0 && position.AvgCostPrice > 0 {
		algo.Market.AverageCost = position.AvgCostPrice
	} else if position.CurrentQty == 0 {
		algo.Market.AverageCost = 0
	}
	log.Println("AvgCostPrice", algo.Market.AverageCost, "Quantity", algo.Market.QuoteAsset.Quantity)
	if firstPositionUpdate {
		shouldHaveQuantity = algo.Market.QuoteAsset.Quantity
		firstPositionUpdate = false
	}
	// algo.logState()
}

func (algo *Algo) updateAlgoBalances(balances []iex.WSBalance) {
	for i := range balances {
		if balances[i].Asset == algo.Market.BaseAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.BaseAsset.Quantity {
				algo.Market.BaseAsset.Quantity = walletAmount
				fmt.Printf("BaseAsset: %+v \n", walletAmount)
			}
		} else if balances[i].Asset == algo.Market.QuoteAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.QuoteAsset.Quantity {
				algo.Market.QuoteAsset.Quantity = walletAmount
				fmt.Printf("QuoteAsset: %+v \n", walletAmount)
			}
		}
	}
}

func (algo *Algo) updateState(ex iex.IExchange, trade iex.TradeBin, localBars *[]*models.Bar, setupData func([]*models.Bar, Algo)) {
	log.Println("Trade Update:", trade)
	algo.Market.Price = trade.Close
	//TODO this is delayed by 1 min -> when we ask the database for 1m bars it returns the previous minute
	data.UpdateLocalBars(localBars, data.GetData("XBTUSD", algo.DecisionInterval, 2))
	setupData(*localBars, *algo)
	algo.Index = len(*localBars) - 1
	algo.Timestamp = time.Now().Truncate(time.Second).UTC().String()
	log.Println("algo.Index", algo.Index)
	if firstTrade {
		algo.logState()
		firstTrade = false
	}
	// Update active option contracts from API
	if algo.Market.Options != nil {
		markets, err := ex.GetMarkets(algo.Market.BaseAsset.Symbol, "option")
		if err != nil {
			fmt.Printf("Got markets from API: %v\n", markets)
			for _, market := range markets {
				containsSymbol := false
				for _, option := range algo.Market.Options {
					if option.Symbol == market.Symbol {
						containsSymbol = true
					}
				}
				if !containsSymbol {
					expiry := market.Expiry * 1000
					optionTheo := models.NewOptionTheo(market.OptionType, algo.Market.Price, market.Strike, ToIntTimestamp(algo.Timestamp), expiry, 0, -1, -1)
					optionContract := models.OptionContract{
						Symbol:           market.Symbol,
						Strike:           market.Strike,
						Expiry:           expiry,
						OptionType:       market.OptionType,
						AverageCost:      0,
						Profit:           0,
						TickSize:         market.TickSize,
						MakerFee:         market.MakerCommission,
						TakerFee:         market.TakerCommission,
						MinimumOrderSize: market.MinTradeAmount,
						Position:         0,
						OptionTheo:       *optionTheo,
						Status:           "open",
					}
					algo.Market.Options = append(algo.Market.Options, optionContract)
				}
			}
		}
	}
}

func (algo *Algo) setupOrders() {
	if algo.AutoOrderPlacement {
		orderSize, side := algo.getOrderSize(algo.Market.Price)
		var quantity float64
		if algo.Market.Futures {
			quantity = orderSize * (algo.Market.BaseAsset.Quantity * algo.Market.Price)
		} else {
			quantity = orderSize * (algo.Market.BaseAsset.Quantity / algo.Market.Price)
		}

		// Keep track of what we should have so the orders we place will grow and shrink
		if shouldHaveQuantity == 0 {
			shouldHaveQuantity = quantity * side
		} else {
			shouldHaveQuantity += quantity * side
		}

		// Get the difference of what we have and what we should have, thats what we should order
		quantityToOrder := shouldHaveQuantity - algo.Market.QuoteAsset.Quantity

		// Don't over order while adding to the position
		orderSide := math.Copysign(1, quantityToOrder)
		quantitySide := math.Copysign(1, algo.Market.QuoteAsset.Quantity)

		if orderSide == quantitySide && math.Abs(shouldHaveQuantity) > algo.canBuy() {
			shouldHaveQuantity = algo.canBuy() * quantitySide
		}

		// Don't over order to go neutral
		if algo.Market.Weight == 0 && math.Abs(quantityToOrder) > math.Abs(algo.Market.QuoteAsset.Quantity) {
			shouldHaveQuantity = 0
			quantityToOrder = -algo.Market.QuoteAsset.Quantity
		}
		log.Println("Can Buy", algo.canBuy(), "shouldHaveQuantity", shouldHaveQuantity, "side", side, "quantityToOrder", quantityToOrder)

		if side == 1 {
			algo.Market.BuyOrders = models.OrderArray{
				Quantity: []float64{math.Abs(quantityToOrder)},
				Price:    []float64{algo.Market.Price - algo.Market.TickSize},
			}
		} else if side == -1 {
			algo.Market.SellOrders = models.OrderArray{
				Quantity: []float64{math.Abs(quantityToOrder)},
				Price:    []float64{algo.Market.Price + algo.Market.TickSize},
			}
		}

	} else {
		if algo.Market.Futures {
			algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying * algo.Market.Price))
			algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling * algo.Market.Price))
		} else {
			algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying / algo.Market.Price))
			algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling / algo.Market.Price))
		}
	}
}
