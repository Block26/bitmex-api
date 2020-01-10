package yantra

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/data"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
)

var orderStatus iex.OrderStatus
var firstTrade bool
var firstPositionUpdate bool
var shouldHaveQuantity float64
var commitHash string

// Connect is called to connect to an exchanges WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of Algo.RebalanceInterval
//
// This is intentional, look at Algo.AutoOrderPlacement to understand this paradigm.
func Connect(settingsFile string, secret bool, algo Algo, rebalance func(Algo) Algo, setupData func([]*models.Bar, Algo)) {
	if algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}
	firstTrade = true
	firstPositionUpdate = true
	config := utils.LoadConfiguration(settingsFile, secret)
	log.Printf("Loaded config for %v \n", algo.Market.Exchange)
	commitHash = time.Now().String()

	exchangeVars := iex.ExchangeConf{
		Exchange:       algo.Market.Exchange,
		ServerUrl:      algo.Market.ExchangeURL,
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		log.Println(err)
	}
	orderStatus = ex.GetPotentialOrderStatus()

	log.Printf("Getting data with symbol %v, decisioninterval %v, datalength %v\n", algo.Market.Symbol, algo.RebalanceInterval, algo.DataLength+1)
	localBars := data.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, algo.DataLength+1)
	log.Printf("Got local bars: %v\n", len(localBars))
	// log.Println(len(localBars), "downloaded")

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := ex.GetBalances()
	algo.updateAlgoBalances(balances)

	positions, _ := ex.GetPositions(algo.Market.BaseAsset.Symbol)
	algo.updatePositions(positions)
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
		log.Println(err)
	}

	var localOrders []iex.WSOrder

	for {
		select {
		case positions := <-channels.PositionChan:
			algo.updatePositions(positions)
		case trade := <-channels.TradeBinChan:
			algo.updateState(ex, trade[0], localBars, setupData)
			algo = rebalance(algo)
			algo.setupOrders()
			algo.placeOrdersOnBook(ex, localOrders)
			algo.logState()
		case newOrders := <-channels.OrderChan:
			// log.Println("update channels.OrderChan")
			// log.Printf("Got new websocket orders: %v\n", newOrders)
			localOrders = updateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			algo.updateAlgoBalances(update.Balance)
		}
	}
}

func (algo *Algo) updatePositions(positions []iex.WsPosition) {
	log.Println("Position Update:", positions)
	if len(positions) > 0 {
		for _, position := range positions {
			if position.Symbol == algo.Market.QuoteAsset.Symbol {
				algo.Market.QuoteAsset.Quantity = float64(position.CurrentQty)
				if math.Abs(algo.Market.QuoteAsset.Quantity) > 0 && position.AvgCostPrice > 0 {
					algo.Market.AverageCost = position.AvgCostPrice
				} else if position.CurrentQty == 0 {
					algo.Market.AverageCost = 0
				}
				log.Println("AvgCostPrice", algo.Market.AverageCost, "Quantity", algo.Market.QuoteAsset.Quantity)
			} else if position.Symbol == algo.Market.BaseAsset.Symbol {
				algo.Market.BaseAsset.Quantity = float64(position.CurrentQty)
				log.Println("BaseAsset updated")
			} else {
				for i := range algo.Market.OptionContracts {
					option := &algo.Market.OptionContracts[i]
					if option.Symbol == position.Symbol {
						option.Position = position.CurrentQty
						option.AverageCost = position.AvgCostPrice
						log.Printf("[%v] Updated position %v, average cost %v\n", option.Symbol, option.Position, option.AverageCost)
						break
					}
				}
			}
		}
		if firstPositionUpdate {
			algo.logState()
			shouldHaveQuantity = algo.Market.QuoteAsset.Quantity
			firstPositionUpdate = false
		}
	}
	algo.logState()
}

func (algo *Algo) updateAlgoBalances(balances []iex.WSBalance) {
	for i := range balances {
		if balances[i].Asset == algo.Market.BaseAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.BaseAsset.Quantity {
				algo.Market.BaseAsset.Quantity = walletAmount
				log.Printf("BaseAsset: %+v \n", walletAmount)
			}
		} else if balances[i].Asset == algo.Market.QuoteAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.QuoteAsset.Quantity {
				algo.Market.QuoteAsset.Quantity = walletAmount
				log.Printf("QuoteAsset: %+v \n", walletAmount)
			}
		}
	}
}

func (algo *Algo) updateState(ex iex.IExchange, trade iex.TradeBin, localBars []*models.Bar, setupData func([]*models.Bar, Algo)) {
	log.Println("Trade Update:", trade)
	localBars = data.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, 2)
	algo.Index = len(localBars) - 1
	algo.Market.Price = utils.ConvertTradeBinToBar(trade)
	setupData(localBars, *algo)
	algo.Timestamp = time.Unix(localBars[algo.Index].Timestamp/1000, 0).UTC().String()
	log.Println("algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if firstTrade {
		algo.logState()
		firstTrade = false
	}
	// Update active option contracts from API
	if algo.Market.Exchange == "deribit" && algo.Market.Options {
		markets, err := ex.GetMarkets(algo.Market.BaseAsset.Symbol, true, "option")
		if err == nil {
			// log.Printf("Got markets from API: %v\n", markets)
			for _, market := range markets {
				containsSymbol := false
				for _, option := range algo.Market.OptionContracts {
					if option.Symbol == market.Symbol {
						containsSymbol = true
					}
				}
				if !containsSymbol {
					// expiry := market.Expiry * 1000
					expiry := market.Expiry
					optionTheo := models.NewOptionTheo(market.OptionType, algo.Market.Price.Close, market.Strike, utils.ToIntTimestamp(algo.Timestamp), expiry, 0, -1, -1)
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
						MidMarketPrice:   market.MidMarketPrice,
					}
					log.Printf("Set mid market price for %v: %v\n", market.Symbol, market.MidMarketPrice)
					algo.Market.OptionContracts = append(algo.Market.OptionContracts, optionContract)
				}
			}
		} else {
			log.Printf("Error getting markets: %v\n", err)
		}
	}
}
