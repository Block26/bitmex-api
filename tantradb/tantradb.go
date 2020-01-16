package tantradb

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"

	_ "github.com/lib/pq"
	"github.com/tantralabs/yantra/models"
)

const env = ""

var (
	host     = "localhost"
	port     = 5433
	user     = "yantrauser"
	password = "password"
	dbname   = "tantra"
)

func Setup(env string) {
	if env == "remote" {
		host = "tantradb.czzctnxje5ad.us-west-1.rds.amazonaws.com"
		port = 5432
		user = "yantrauser"
		password = "soncdw0csxvpWUHDQNksamsda"
		dbname = "template1"
		fmt.Printf("Set up remote tantradb with URL %v\n", host)
	} else {
		fmt.Printf("Using local tantradb.\n")
	}
}

func LoadImpliedVols(symbol string, start int, end int) []models.ImpliedVol {
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
