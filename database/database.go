// Package data handles database connections and data gathering.
package database

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
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

// GetData is called to fetch data from your local psql database setup by https://github.com/tantralabs/tantradb
func GetDataByTime(symbol string, exchange string, interval string, startTimestamp time.Time, endTimestamp time.Time) []*models.Bar {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return GetDataByTime(symbol, exchange, interval, startTimestamp, endTimestamp)
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

// GetData is called to fetch data from your local psql database setup by https://github.com/tantralabs/tantradb
func GetData(symbol string, exchange string, interval string, numBars int) []*models.Bar {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Connect("postgres", psqlInfo)

	if err != nil {
		if host == "localhost" {
			log.Println("Falied to connect to database, attempting to connect to cloud database. Please setup tantradb locally.")
			Setup("remote")
			return GetData(symbol, exchange, interval, numBars)
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

func LoadImpliedVols(symbol string, start int, end int) []models.ImpliedVol {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sqlx.Connect("postgres", psqlInfo)

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
	// fmt.Printf("Command: %v", cmd)
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
