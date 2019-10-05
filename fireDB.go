package algo

import (
	"context"
	"log"
	"time"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/db"
	"google.golang.org/api/option"
)

type AlgoInfo struct {
	BtcBalance         float64 `json:"btc_balance,omitempty"`
	UsdBalance         float64 `json:"usd_balance,omitempty"`
	StartingBtcBalance float64 `json:"starting_balance_btc,omitempty"`
	StartingUsdBalance float64 `json:"starting_balance_usd,omitempty"`
}

func setupFirebase() *db.Client {
	ctx := context.Background()
	conf := &firebase.Config{
		DatabaseURL: "https://algostats-95d95.firebaseio.com",
	}
	// Fetch the service account key JSON file contents
	opt := option.WithCredentialsFile("./settings/firebase_key.json")

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)
	if err != nil {
		log.Fatalln("Error initializing app:", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		log.Fatalln("Error initializing database client:", err)
	}

	log.Println(client)
	return client

	// // Get a database reference to our posts
	// ref := client.NewRef("algos/live/mm")

	// // Read the data at the posts reference (this is a blocking operation)
	// var info AlgoInfo
	// if err := ref.Get(ctx, &info); err != nil {
	// 	log.Fatalln("Error reading value:", err)
	// }
	// log.Println(info)
}

func updateAlgo(client *db.Client, algoName string) {
	// // Get a database reference to our posts
	ctx := context.Background()
	ref := client.NewRef("algos/live/" + algoName + "/last_update")
	now := time.Now().Unix()
	ref.Set(ctx, now)
	// Read the data at the posts reference (this is a blocking operation)
	// var info AlgoInfo
	// if err := ref.Get(ctx, &info); err != nil {
	// 	log.Fatalln("Error reading value:", err)
	// }
	// log.Println(info)
}
