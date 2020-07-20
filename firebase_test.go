package yantra

import (
	"context"
	"fmt"
	"log"
	"testing"

	firebase "firebase.google.com/go"
	"github.com/tantralabs/yantra/utils"
	"google.golang.org/api/option"
)

func TestLogToFirebase(t *testing.T) {
	ctx := context.Background()

	conf := &firebase.Config{
		DatabaseURL: "https://live-algos.firebaseio.com",
	}

	file := utils.DownloadFirebaseCreds()
	opt := option.WithCredentialsFile(file.Name())

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)

	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		fmt.Println("Error connecting to db:", err)
	}

	ref := client.NewRef("live/test-algo")

	err = ref.Set(ctx, map[string]interface{}{
		"leverage": 0.5,
		"symbol":   "BTC-PERPETUAL",
		"exchange": "deribit",
	})
	if err != nil {
		fmt.Println("Error setting value:", err)
	}
}

func TestReadFromFirebase(t *testing.T) {
	ctx := context.Background()

	conf := &firebase.Config{
		DatabaseURL: "https://live-algos.firebaseio.com",
	}

	file := utils.DownloadFirebaseCreds()
	opt := option.WithCredentialsFile(file.Name())

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)

	if err != nil {
		fmt.Println("error initializing app:", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		fmt.Println("Error connecting to db:", err)
	}

	ref := client.NewRef("live/")
	var leverages interface{}
	if err := ref.Get(ctx, &leverages); err != nil {
		fmt.Println("Error reading value:", err)
	}

	fmt.Println(leverages)
}
