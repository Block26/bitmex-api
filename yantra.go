package yantra

import (
	"context"
	"fmt"
	"log"

	firebase "firebase.google.com/go"
	"github.com/tantralabs/logger"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
	"google.golang.org/api/option"
)

func CreateNewAlgo(config models.AlgoConfig) models.Algo {
	exchangeInfo, err := exchanges.LoadExchangeInfo(config.Exchange)
	exchangeInfo.OptionStrikeInterval = 500
	if err != nil {
		log.Fatal(err)
	}
	account := models.NewAccount(config.Symbol, exchangeInfo, config.StartingBalance)
	logger.Infof("Got account with id %v: %v\n", account.AccountID, account)
	logger.Infof("Loaded market info with symbol %v\n", account.BaseAsset.Symbol)
	liveConfig := models.LoadConfig("config.json")
	if config.SharpeCalculationInterval == 0 {
		config.SharpeCalculationInterval = 7
	}
	return models.Algo{
		Name:                      config.Name,
		Account:                   account,
		DataLength:                config.DataLength,
		RebalanceInterval:         config.RebalanceInterval,
		SharpeCalculationInterval: config.SharpeCalculationInterval,
		LogBacktest:               config.LogBacktest,
		LogCloudBacktest:          config.LogCloudBacktest,
		LogLevel:                  logger.LogLevel().Debug,
		BacktestLogLevel:          logger.LogLevel().Info,
		State:                     make(map[string]interface{}, 0),
		Config:                    liveConfig,
		Rebalance:                 config.Rebalance,
		SetupData:                 config.SetupData,
		OnOrderUpdate:             config.OnOrderUpdate,
		OnPositionUpdate:          config.OnPositionUpdate,
	}
}

func GetAllAlgoStatus() (status map[string]models.AlgoStatus) {
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
	if err := ref.Get(ctx, &status); err != nil {
		fmt.Println("Error reading value:", err)
	}

	logger.Debug("Algo Status", status)
	return
}

func GetSelectAlgoStatus(algos []string) map[string]models.AlgoStatus {
	status := GetAllAlgoStatus()

	selected := make(map[string]models.AlgoStatus)
	for name, algoStatus := range status {
		for _, algoName := range algos {
			if name == algoName {
				selected[name] = algoStatus
			}
		}
	}

	logger.Debug("Selected Algo Status", selected)
	return selected
}
