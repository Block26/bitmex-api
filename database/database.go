// Package data handles database connections and data gathering.
package database

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tantralabs/logger"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/models"
)

var (
	host     = "localhost"
	port     = 5432
	user     = "yantrauser"
	password = "password"
	dbname   = "tantra"
	ENV      = ""
)

var knownHistory []TransactionRecord = make([]TransactionRecord, 0)

func Setup(env ...string) {
	if env != nil && env[0] != ENV {
		ENV = env[0]
	}
	if ENV == "remote" {
		host = "tantradb.czzctnxje5ad.us-west-1.rds.amazonaws.com"
		port = 5432
		user = "yantrauser"
		password = "soncdw0csxvpWUHDQNksamsda"
		dbname = "tantra"
	}
}

func GetDB() *sqlx.DB {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)
	if err != nil {
		logger.Errorf("Error connecting to db: %v\n", err)
	}
	return db
}

func NewDB() *sqlx.DB {
	db := GetDB()
	ResetTables(db)
	return db
}

func Quote(str string) string {
	return "'" + str + "'"
}

func QuoteF(quantity float64) string {
	return Quote(strconv.FormatFloat(quantity, 'f', -1, 64))
}

func QuoteI(quantity int) string {
	return Quote(strconv.FormatInt(int64(quantity), 10))
}

func ResetTables(db *sqlx.DB) {
	query := "drop table if exists market_state_snapshots;"
	db.MustExec(query)
	query = "drop table if exists algo_snapshots;"
	db.MustExec(query)
	query = "drop table if exists trades;"
	db.MustExec(query)
	query = "drop table if exists account_snapshots;"
	db.MustExec(query)
	query = "drop table if exists market_state_snapshots;"
	db.MustExec(query)
	SetupAlgoSnapshotTable(db)
	SetupMarketSnapshotTable(db)
	SetupTradeTable(db)
	SetupAccountSnapshotTable(db)
}

func SetupAccountSnapshotTable(db *sqlx.DB) {
	query := fmt.Sprintf(`
	create table if not exists account_snapshots(
		id serial primary key,
		timestamp bigint not null,
		unrealized_profit double precision not null,
		realized_profit double precision not null,
		profit double precision not null
	);`)
	db.MustExec(query)
}

func SetupAlgoSnapshotTable(db *sqlx.DB) {
	query := fmt.Sprintf(`
	create table if not exists algo_snapshots(
		id serial primary key,
		timestamp bigint not null,
		symbol text not null,
		price double precision not null,
		balance double precision not null,
		average_cost double precision,
		profit double precision,
		option_profit double precision,
		position double precision,
		leverage double precision,
		delta double precision,
		gamma double precision,
		theta double precision,
		vega double precision,
		wvega double precision,
		premium double precision
	);`)
	db.MustExec(query)
}

func SetupMarketSnapshotTable(db *sqlx.DB) {
	query := fmt.Sprintf(`
	create table if not exists market_state_snapshots(
		id serial primary key,
		timestamp bigint not null,
		symbol text not null,
		balance double precision,
		average_cost double precision,
		unrealized_profit double precision,
		realized_profit double precision,
		open double precision,
		high double precision,
		low double precision,
		close double precision,
		volume double precision,
		strike double precision,
		type text,
		expiry bigint,
		theo double precision,
		delta double precision,
		gamma double precision,
		theta double precision,
		vega double precision,
		wvega double precision,
		volatility double precision,
		position double precision
	);`)
	db.MustExec(query)
}

func SetupTradeTable(db *sqlx.DB) {
	query := fmt.Sprintf(`
	create table if not exists trades(
		id serial primary key,
		trade_id text not null,
		symbol text not null,
		amount double precision not null,
		price double precision not null,
		side text not null,
		maker_id text,
		taker_id text,
		timestamp bigint not null
	);`)
	db.MustExec(query)
}

func GetCandlesByTime(symbol string, exchange string, interval string, startTimestamp time.Time, endTimestamp time.Time) []*models.Bar {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return GetCandlesByTime(symbol, exchange, interval, startTimestamp, endTimestamp)
		} else {
			log.Fatal(err)
		}
	}

	bars := []*models.Bar{}
	cmd := fmt.Sprintf("select timestamp, open, high, low, close, volume from candles where symbol = '%s' and exchange = '%s' and interval = '%s' and timestamp >= '%d' and timestamp <= '%d';", symbol, exchange, interval, startTimestamp.Unix()*1000, endTimestamp.Unix()*1000)
	err = db.Select(&bars, cmd)
	if err != nil {
		log.Fatal(err)
	}

	if len(bars) == 0 {
		log.Fatal("There doesn't seem to be any data for ", exchange, " ", symbol, " on the ", interval, " interval in the database. Maybe it was your start and end dates?")
	}

	db.Close()
	sort.Slice(bars, func(i, j int) bool { return bars[i].Timestamp < bars[j].Timestamp })
	return bars
}

func GetCandlesByTimeWithBuffer(symbol string, exchange string, interval string, startTimestamp time.Time, endTimestamp time.Time, numPrepending int) []*models.Bar {
	logger.Infof("Getting data by time for symbol %v with start %v, end %v, num prepending %v\n", symbol, startTimestamp, endTimestamp, numPrepending)
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	logger.Infof("Getting candles with pg connect string: %v\n", psqlInfo)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			logger.Errorf("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return GetCandlesByTimeWithBuffer(symbol, exchange, interval, startTimestamp, endTimestamp, numPrepending)
		} else {
			log.Fatal(err)
		}
	}
	if numPrepending > 0 {
		oldStart := startTimestamp
		var freq time.Duration
		if interval == "1m" {
			freq = time.Minute
		} else if interval == "1h" {
			freq = time.Hour
		} else if interval == "1d" {
			freq = time.Hour * 24
		} else {
			log.Fatal("Unsupported interval for db: ", interval)
		}
		startTimestamp = startTimestamp.Add(-freq * time.Duration(numPrepending))
		logger.Infof("Start before prepend: %v, after: %v\n", oldStart, startTimestamp)
	}

	bars := []*models.Bar{}
	cmd := fmt.Sprintf("select timestamp, open, high, low, close, volume from candles where symbol = '%s' and exchange = '%s' and interval = '%s' and timestamp >= '%d' and timestamp <= '%d';", symbol, exchange, interval, startTimestamp.Unix()*1000, endTimestamp.Unix()*1000)
	err = db.Select(&bars, cmd)
	if err != nil {
		log.Fatal(err)
	}

	if len(bars) == 0 {
		log.Fatal("There doesn't seem to be any data for ", exchange, " ", symbol, " on the ", interval, " interval in the database. Maybe it was your start and end dates?")
	}

	db.Close()
	sort.Slice(bars, func(i, j int) bool { return bars[i].Timestamp < bars[j].Timestamp })
	return bars
}

func GetCandles(symbol string, exchange string, interval string, numBars int) []*models.Bar {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return GetCandles(symbol, exchange, interval, numBars)
		} else {
			log.Fatal(err)
		}
	}

	bars := []*models.Bar{}
	cmd := fmt.Sprintf("select timestamp, open, high, low, close, volume from candles where symbol = '%s' and exchange = '%s' and interval = '%s' order by timestamp desc limit %s;", symbol, exchange, interval, strconv.Itoa(numBars))
	err = db.Select(&bars, cmd)
	if err != nil {
		log.Fatal(err)
	}

	if len(bars) == 0 {
		log.Fatal("There doesn't seem to be any data for ", exchange, " ", symbol, " on the ", interval, " interval in the database. Maybe it was your start and end dates?")
	}

	db.Close()
	sort.Slice(bars, func(i, j int) bool { return bars[i].Timestamp < bars[j].Timestamp })
	return bars
}

// TODO reconstruct candles for higher intervals
func InsertCandle(candle iex.TradeBin, interval, exchange string) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)
	if err != nil {
		log.Fatal(err)
	}
	query := fmt.Sprintf(`insert into candles(timestamp, open, high, low, close, volume, exchange, symbol, interval) 
		values('%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v')`, candle.Timestamp, candle.Open, candle.High, candle.Low,
		candle.Close, candle.Volume, exchange, candle.Symbol, interval)
	db.MustExec(query)
}

func LoadImpliedVols(symbol string, start int, end int) []models.ImpliedVol {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sqlx.Connect("postgres", psqlInfo)

	logger.Infof("Getting implied vols with pg connect string: %v\n", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return LoadImpliedVols(symbol, start, end)
		} else {
			log.Fatal(err)
		}
	}

	ivs := []models.ImpliedVol{}
	cmd := fmt.Sprintf("select symbol, iv, timestamp, interval, indexprice, vwiv, strike, timetoexpiry, volume from impliedvol where symbol = '%s' and timestamp >= %d and timestamp <= %d order by timestamp\n", symbol, start, end)
	logger.Infof("Vol query: %v\n", cmd)
	err = db.Select(&ivs, cmd)

	if err != nil {
		log.Fatal(err)
	}

	db.Close()
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].Timestamp < ivs[j].Timestamp })
	return ivs
}

// GetData is called to fetch data from your local psql database setup by https://github.com/tantralabs/tantradb
func LogWalletHistory(algo *models.Algo, accountId string, history []iex.WalletHistoryItem) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			LogWalletHistory(algo, accountId, history)
		} else {
			log.Fatal(err)
		}
	}

	newHistory := make([]TransactionRecord, 0)
	for _, item := range history {
		known := false
		for _, record := range knownHistory {
			if item.TxID == record.TxId {
				known = true
				break
			}
		}
		if !known {
			newRecord := TransactionRecord{
				Timestamp: item.TimeStamp,
				Type:      item.TxType,
				Exchange:  algo.ExchangeInfo.Exchange,
				AccountId: accountId,
				AlgoName:  algo.Name,
				TxId:      item.TxID,
				Status:    item.Status,
				Currency:  item.Currency,
				Amount:    item.Amount,
			}
			newHistory = append(newHistory, newRecord)
			knownHistory = append(knownHistory, newRecord)
		}
	}

	tx, err := db.Begin()
	for _, r := range newHistory {
		_, err = db.Exec("insert into wallet_history(timestamp, type, account_id, algo_name, tx_id, status, currency, amount) values ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING;", r.Timestamp, r.Type, r.AccountId, r.AlgoName, r.TxId, r.Status, r.Currency, r.Amount)
		if err != nil {
			fmt.Println("err", err)
		}
	}
	err = tx.Commit()
	if err != nil {
		fmt.Println(err)
	}

	db.Close()
}

func getOptionInsertString(market models.MarketHistory) (insertString string) {
	insertString = fmt.Sprintf(`(%v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v)`,
		QuoteI(market.Timestamp), Quote(market.Symbol), QuoteF(market.Balance), QuoteF(market.AverageCost), QuoteF(market.UnrealizedProfit),
		QuoteF(market.RealizedProfit), QuoteF(market.Strike), QuoteI(int(market.OptionType)),
		QuoteI(market.Expiry), QuoteF(market.Theo), QuoteF(market.Delta), QuoteF(market.Gamma),
		QuoteF(market.Theta), QuoteF(market.Vega), QuoteF(market.WeightedVega),
		QuoteF(market.Volatility), QuoteF(market.Position))
	return
}

func getFutureInsertString(market models.MarketHistory) (insertString string) {
	insertString = fmt.Sprintf(`
	(%v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v, %v)`,
		QuoteI(market.Timestamp), Quote(market.Symbol), QuoteF(market.Balance), QuoteF(market.AverageCost),
		QuoteF(market.UnrealizedProfit), QuoteF(market.RealizedProfit), QuoteF(market.Position),
		QuoteF(market.Open), QuoteF(market.High), QuoteF(market.Low), QuoteF(market.Close), QuoteF(market.Volume))
	return
}

func getAccountInsertString(account models.AccountHistory) (insertString string) {
	insertString = fmt.Sprintf(`(%v, %v, %v, %v)`,
		QuoteI(account.Timestamp), QuoteF(account.RealizedProfit), QuoteF(account.UnrealizedProfit), QuoteF(account.Profit))
	return
}

func getTradeInsertString(trade models.Trade) (insertString string) {
	insertString = fmt.Sprintf(`(%v, %v, %v, %v, %v, %v, %v, %v)`,
		Quote(trade.ID), Quote(trade.Symbol), QuoteF(trade.Amount), QuoteF(trade.Price),
		Quote(trade.Side), Quote(trade.MakerID), Quote(trade.TakerID), QuoteI(trade.Timestamp))
	return
}

func InsertAccountHistory(db *sqlx.DB, accounts []models.AccountHistory) {
	logger.Debugf("Inserting account history for %v records...\n", len(accounts))
	var insertStrings []string
	var query string
	var err error
	for _, account := range accounts {
		insertStrings = append(insertStrings, getAccountInsertString(account))
	}
	query = fmt.Sprintf(`
	insert into account_snapshots(timestamp, unrealized_profit, realized_profit, profit)
	values %v;`, strings.Join(insertStrings, ","))
	_, err = db.Exec(query)
	if err != nil {
		logger.Errorf("Error inserting account snapshots: %v\n", err)
	} else {
		logger.Infof("Inserted %v account snapshots.\n", len(insertStrings))
	}
}

func InsertMarketHistory(db *sqlx.DB, markets []map[string]models.MarketHistory) {
	logger.Debugf("Inserting market history for %v records...\n", len(markets))
	var insertStrings []string
	var query string
	var err error
	// Insert futures histories first
	for _, marketMap := range markets {
		for _, market := range marketMap {
			if market.MarketType != models.Option {
				insertStrings = append(insertStrings, getFutureInsertString(market))
			}
		}
	}
	query = fmt.Sprintf(`
		insert into market_state_snapshots(timestamp, symbol, balance, average_cost, unrealized_profit, realized_profit, position, open, high, low, close, volume)
		values %v;`, strings.Join(insertStrings, ","))
	_, err = db.Exec(query)
	if err != nil {
		logger.Errorf("Error inserting future snapshots: %v\n", err)
	} else {
		logger.Infof("Inserted %v future snapshots.\n", len(insertStrings))
	}
	// Insert option histories
	insertStrings = nil
	var insertString string
	for _, marketMap := range markets {
		for _, market := range marketMap {
			if market.MarketType == models.Option {
				insertString = getOptionInsertString(market)
				if len(insertString) > 0 {
					insertStrings = append(insertStrings, insertString)
				}
			}
		}
	}
	if len(insertStrings) > 0 {
		query = fmt.Sprintf(`
		insert into market_state_snapshots(timestamp, symbol, balance, average_cost, unrealized_profit, realized_profit, strike, type, expiry, theo, delta, gamma, theta, vega, wvega, volatility, position)
		values %v;`, strings.Join(insertStrings, ","))
		_, err = db.Exec(query)
		if err != nil {
			logger.Errorf("Error inserting option snapshots: %v\n", err)
		} else {
			logger.Infof("Inserted %v option snapshots.\n", len(insertStrings))
		}
	} else {
		logger.Errorf("No option entries to insert.\n")
	}
}

func InsertTradeHistory(db *sqlx.DB, tradeHistory []models.Trade) {
	logger.Debugf("Inserting trade history for %v records...\n", len(tradeHistory))
	var insertStrings []string
	var query string
	var err error
	// Insert futures histories first
	for _, trade := range tradeHistory {
		insertStrings = append(insertStrings, getTradeInsertString(trade))
	}
	query = fmt.Sprintf(`
		insert into trades(trade_id, symbol, amount, price, side, maker_id, taker_id, timestamp)
		values %v;`, strings.Join(insertStrings, ","))
	logger.Debugf("Trade insert query: %v\n", query)
	_, err = db.Exec(query)
	if err != nil {
		logger.Errorf("Error inserting trades: %v\n", err)
	} else {
		logger.Infof("Inserted %v trades.\n", len(insertStrings))
	}
}
