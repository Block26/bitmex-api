# Yantra
Follow the wiki to get started

## Documentation

Documentation is provided by `godoc`. To run it locally, install godoc (ubuntu):

`sudo apt install golang-go.tools`

Then, you can do `godoc -http=:6060` and open `localhost:6060` in your browser. Godoc uses your GOROOT environment variable for URL navigation, and expects the format to be something like `/home/user/go/src/github.com/tantralabs/yantra` (note the `src` directory). Then, you can navigate to the `yantra` folder in your browser to see the documentation.


## Environment Variables

Set the following environment variables for use with yantra:

YANTRA_BACKTEST_DB_URL -> Influx db url for backtests
YANTRA_BACKTEST_DB_USER -> optional username
YANTRA_BACKTEST_DB_PASSWORD -> optional password

YANTRA_LIVE_DB_URL -> Influx db url for live algos
YANTRA_LIVE_DB_USER -> optional username
YANTRA_LIVE_DB_PASSWORD -> optional password

On ubuntu, you can set these in `~/.bashrc` and run `source ~/.bashrc` to activate them.

