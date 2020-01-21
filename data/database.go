// Package data handles database connections and data gathering.
package data

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
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
		fmt.Printf("Set up remote db with URL %v\n", host)
	} else {
		fmt.Printf("Using local db.")
	}
}

// GetData is called to fetch data from your local psql database setup by https://github.com/tantralabs/tantradb
func GetData(symbol string, exchange string, interval string, startTimestamp time.Time, endTimestamp time.Time) []*models.Bar {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Open("postgres", psqlInfo)

	if err != nil {
		log.Fatal(err)
	}

	bars := []*models.Bar{}
	cmd := fmt.Sprintf("select timestamp, open, high, low, close, volume from candles where symbol = '%s' and exchange = '%s' and interval = '%s' and timestamp >= '%d' and timestamp <= '%d'", symbol, exchange, interval, startTimestamp.Unix()*1000, endTimestamp.Unix()*1000)
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
	Setup()
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sqlx.Connect("postgres", psqlInfo)
	if err != nil {
		fmt.Printf("Error connecting to tantradb: %v\n", err)
		panic(err)
	}

	ivs := []models.ImpliedVol{}
	cmd := fmt.Sprintf("select symbol, iv, timestamp, interval, indexprice, vwiv, strike, timetoexpiry, volume from impliedvol where timestamp >= %d and timestamp <= %d order by timestamp\n", start, end)
	fmt.Printf("Command: %v", cmd)
	err = db.Select(&ivs, cmd)

	if err != nil {
		log.Fatal(err)
	}

	db.Close()
	return ivs
}
