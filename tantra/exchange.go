package tantra

import (
	"errors"
	"time"

	"github.com/tantralabs/database"
	"github.com/tantralabs/models"
	"github.com/tantralabs/tradeapi/global/clients"
	"github.com/tantralabs/tradeapi/iex"
)

var exchangeToMock string
var marketModel models.Market
var index int
var data []iex.TradeBin
var start time.Time
var end time.Time

// New return a instanciate bittrex struct
func New(vars iex.ExchangeConf, market models.Market) *Tantra {
	client := clients.NewClient(vars)
	exchangeToMock = vars.Exchange
	marketModel = market
	return &Tantra{client: client}
}

// Tantra represent a Tantra client
type Tantra struct {
	client *clients.Client
	iex.IExchange
	channels *iex.WSChannels
}

func (t *Tantra) StartWS(config interface{}) error {
	conf, ok := config.(*iex.WsConfig)
	if !ok {
		return errors.New("Assertion failed: config")
	}

	t.channels = conf.Channels

	go func() {
		for i := range []int{0, 1, 2} {
			marketModel.BaseAsset.Quantity = marketModel.BaseAsset.Quantity * 1.3
			wallet := iex.WSWallet{
				Balance: []iex.WSBalance{
					{
						Asset:   marketModel.BaseAsset.Symbol,
						Balance: marketModel.BaseAsset.Quantity,
					},
				},
			}
			t.channels.WalletChan <- &wallet

			if i == 0 {
				pos := []iex.WsPosition{
					iex.WsPosition{
						Symbol:       marketModel.QuoteAsset.Symbol,
						AvgCostPrice: marketModel.AverageCost,
						CurrentQty:   marketModel.QuoteAsset.Quantity,
					},
				}
				t.channels.PositionChan <- pos
			}
		}
	}()

	return nil
}

func (t *Tantra) GetPotentialOrderStatus() iex.OrderStatus {
	return iex.OrderStatus{
		Filled:    "Filled",
		Rejected:  "Rejected",
		Cancelled: "Canceled",
	}
}

func (t *Tantra) GetData(symbol string, binSize string, amount int) ([]iex.TradeBin, error) {
	// only fetch data the first time
	if data == nil {
		start = time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC)
		end = time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
		bars := database.GetData(symbol, exchangeToMock, binSize, start, end)
		for i := range bars {
			tb := iex.TradeBin{
				Open:      bars[i].Open,
				High:      bars[i].High,
				Low:       bars[i].Low,
				Close:     bars[i].Close,
				Volume:    bars[i].Volume,
				Timestamp: time.Unix(bars[i].Timestamp, 0),
			}
			data = append(data, tb)
		}
	}
	return data, nil
}

func (t *Tantra) GetBalances() (balance []iex.WSBalance, err error) {
	balance = append(balance, iex.WSBalance{
		Asset:   marketModel.BaseAsset.Symbol,
		Balance: marketModel.BaseAsset.Quantity,
	})
	return
}

func (t *Tantra) GetPositions(currency string) (positions []iex.WsPosition, err error) {
	pos := iex.WsPosition{
		Symbol:       marketModel.QuoteAsset.Symbol,
		AvgCostPrice: marketModel.AverageCost,
		CurrentQty:   marketModel.QuoteAsset.Quantity,
	}
	positions = append(positions, pos)

	for _, opt := range marketModel.OptionContracts {
		if opt.Position != 0 {
			pos := iex.WsPosition{
				Symbol:       opt.Symbol,
				AvgCostPrice: opt.AverageCost,
				CurrentQty:   opt.Position,
			}
			positions = append(positions, pos)
		}
	}
	return
}

func (t *Tantra) GetOpenOrders(vars iex.OpenOrderF) (orders []iex.Order, err error) {
	// TODO this should return currently open orders
	orders = make([]iex.Order, 0)
	return
}

//WalletHistory not available for this exchange
func (b *Tantra) GetWalletHistory(currency string) (res []iex.WalletHistoryItem, err error) {
	err = errors.New(":error: WalletHistory not available for this exchange yet")
	return
}
