package data

import (
	"fmt"
	"log"
	"strings"

	"github.com/block26/TheAlgoV2/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const (
	host     = "bitmexdata.cevu6a2ct9qj.us-west-1.rds.amazonaws.com"
	port     = 45832
	user     = "b26adminuser"
	password = "R%ED6f^ZP&ddPwEg"
	dbname   = "bmex_data"
)

func GetData(symbol string, bin string, count int) []models.Bar {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sqlx.Open("postgres", psqlInfo)

	if err != nil {
		log.Fatal(err)
	}

	bars := []models.Bar{}
	tableName := strings.ToLower(symbol + "_" + bin)
	cmd := fmt.Sprintf("SELECT timestamp, open, high, low, close, volume FROM public.%s ORDER BY \"timestamp\" DESC LIMIT %d", tableName, count)
	err = db.Select(&bars, cmd)

	if err != nil {
		log.Fatal(err)
	}

	db.Close()

	return bars
}
