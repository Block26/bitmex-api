package tantradb

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"

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

func LoadImpliedVols(symbol string, start int, end int) []models.ImpliedVol {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sqlx.Connect("postgres", psqlInfo)
	if err != nil {
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
