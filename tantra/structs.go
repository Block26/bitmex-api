package tantra

import (
	"encoding/json"
)

type jsonResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type Ticker struct {
	Bid  float64 `json:"Bid"`
	Ask  float64 `json:"Ask"`
	Last float64 `json:"Last"`
}

type Uuid struct {
	Id string `json:"uuid"`
}

type Market struct {
	MarketName        string  `json:"MarketName"`
	High              float64 `json:"High"`
	Low               float64 `json:"Low"`
	Volume            float64 `json:"Volume"`
	Last              float64 `json:"Last"`
	BaseVolume        float64 `json:"BaseVolume"`
	TimeStamp         string  `json:"TimeStamp"`
	Bid               float64 `json:"Bid"`
	Ask               float64 `json:"Ask"`
	OpenBuyOrders     int     `json:"OpenBuyOrders"`
	OpenSellOrders    int     `json:"OpenSellOrders"`
	PrevDay           float64 `json:"PrevDay"`
	Created           string  `json:"Created"`
	DisplayMarketName string  `json:"DisplayMarketName"`
}

type OrderBook struct {
	Buy  []OrderItem `json:"buy"`
	Sell []OrderItem `json:"sell"`
}

type OrderItem struct {
	Quantity float64 `json:"Quantity"`
	Rate     float64 `json:"Rate"`
}

type Balance struct {
	Currency      string  `json:"Currency"`
	Balance       float64 `json:"Balance"`
	Available     float64 `json:"Available"`
	Pending       float64 `json:"Pending"`
	CryptoAddress string  `json:"CryptoAddress"`
	Requested     bool    `json:"Requested"`
	Uuid          string  `json:"Uuid"`
}

type OpenOrder struct {
	CancelInitiated   bool        `json:"CancelInitiated"`
	Closed            interface{} `json:"Closed"`
	CommissionPaid    float64     `json:"CommissionPaid"`
	Condition         interface{} `json:"Condition"`
	ConditionTarget   interface{} `json:"ConditionTarget"`
	Exchange          string      `json:"Exchange"`
	ImmediateOrCancel bool        `json:"ImmediateOrCancel"`
	IsConditional     bool        `json:"IsConditional"`
	Limit             float64     `json:"Limit"`
	Opened            string      `json:"Opened"`
	OrderType         string      `json:"OrderType"`
	OrderUUID         string      `json:"OrderUuid"`
	Price             float64     `json:"Price"`
	PricePerUnit      float64     `json:"PricePerUnit"`
	Quantity          float64     `json:"Quantity"`
	QuantityRemaining float64     `json:"QuantityRemaining"`
	UUID              string      `json:"Uuid"`
}

//GetOrders
type ByOrderRate []OrderItem

func (v ByOrderRate) Len() int      { return len(v) }
func (v ByOrderRate) Swap(i, j int) { v[i], v[j] = v[j], v[i] }
func (v ByOrderRate) Less(i, j int) bool {
	if v[i].Rate < v[j].Rate {
		return true
	}
	if v[i].Rate > v[j].Rate {
		return false
	}
	return v[i].Rate < v[j].Rate
}
