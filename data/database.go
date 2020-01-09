// Package data handles database connections and data gathering.
package data

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tantralabs/yantra/models"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "yantrauser"
	password = "password"
	dbname   = "tantra"
)

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

	return bars
}
