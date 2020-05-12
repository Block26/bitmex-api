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

	return models.Algo{
		Name:              config.Name,
		Account:           account,
		DataLength:        config.DataLength,
		RebalanceInterval: config.RebalanceInterval,
		LogBacktest:       config.LogBacktest,
		LogLevel:          logger.LogLevel().Debug,
		BacktestLogLevel:  logger.LogLevel().Info,
	}
}
