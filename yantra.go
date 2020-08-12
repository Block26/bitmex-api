package yantra

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/db"
	"github.com/tantralabs/logger"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
	"google.golang.org/api/option"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
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
		LogStateHistory:           config.LogStateHistory,
		Config:                    liveConfig,
		Rebalance:                 config.Rebalance,
		SetupData:                 config.SetupData,
		OnOrderUpdate:             config.OnOrderUpdate,
		OnPositionUpdate:          config.OnPositionUpdate,
	}
}

func getFirebaseClient(ctx context.Context) *db.Client {
	conf := &firebase.Config{
		DatabaseURL: "https://live-algos.firebaseio.com",
	}

	file := utils.DownloadFirebaseCreds()
	opt := option.WithCredentialsFile(file.Name())

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)

	if err != nil {
		log.Fatalln("error initializing app:", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		log.Fatalln("Error connecting to db:", err)
	}

	return client
}

func GetPortfolio(portfolioName string) (portfolio models.Portfolio) {
	ctx := context.Background()
	client := getFirebaseClient(ctx)
	ref := client.NewRef("portfolio/" + portfolioName + "/")
	fmt.Println(ref.Path)
	var res map[string]interface{}

	if err := ref.Get(ctx, &res); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	file, _ := json.MarshalIndent(res, "", " ")
	_ = ioutil.WriteFile("portfolio.json", file, 0644)

	// god only knows why golang wouldnt just let me put it straight into my struct
	portfolio = models.LoadPortfolio("portfolio.json")
	return
}

func GetAllAlgoStatus() (status map[string]models.AlgoStatus) {
	ctx := context.Background()
	client := getFirebaseClient(ctx)
	ref := client.NewRef("live/")
	if err := ref.Get(ctx, &status); err != nil {
		log.Fatalln("Error reading value:", err)
	}

	return
}

func GetSelectAlgoStatus(algos []string) map[string]models.AlgoStatus {
	ctx := context.Background()
	client := getFirebaseClient(ctx)
	ref := client.NewRef("live/")

	selected := make(map[string]models.AlgoStatus)
	for _, algoName := range algos {
		status := models.AlgoStatus{}
		if err := ref.Child(algoName).Get(ctx, &status); err != nil {
			log.Fatalln("Error reading value:", err)
		}
		selected[algoName] = status
	}

	return selected
}

// Clone an algo based on config to a local directory
func CloneAlgo(config models.Config) (success bool) {
	dir := "./" + config.Name

	// remove files when local debugging
	os.RemoveAll(dir)
	refName := fmt.Sprintf("refs/heads/%s", config.Branch)

	// Clone Algo to local directory
	r, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           "https://" + config.Algo,
		ReferenceName: plumbing.ReferenceName(refName),
		Auth: &http.BasicAuth{
			Username: "abc123", // yes, this can be anything except an empty string
			Password: os.Getenv("GITHUB_TOKEN"),
		},
		Progress: os.Stdout,
	})

	if err != nil {
		log.Fatalln(err.Error())
	}

	// Get Working Tree
	w, err := r.Worktree()

	if err != nil {
		log.Fatalln("working tree", err.Error())
	}

	// Checkout Algo commit hash
	if config.Commit != "latest" {
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(config.Commit),
		})

		if err != nil {
			log.Fatalln("checkout commit", err.Error())
		}
	}

	// cd to tmp
	os.Chdir(dir)
	// download deps
	run("go", "mod", "download")

	// Go back to parent dir
	os.Chdir("..")
	return
}

func run(app string, args ...string) error {
	log.Println(app, args)
	cmd := exec.Command(app, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
