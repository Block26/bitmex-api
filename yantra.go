package yantra

import (
	"log"

	"github.com/tantralabs/logger"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
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
	}
}
