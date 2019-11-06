module github.com/tantralabs/TheAlgoV2

go 1.13

// replace github.com/tantralabs/exchanges => ../../tantralabs/exchanges

// replace github.com/tantralabs/tradeapi => ../../../github.com/tantralabs/tradeapi

require (
	firebase.google.com/go v3.10.0+incompatible
	github.com/aws/aws-sdk-go v1.25.28
	github.com/gocarina/gocsv v0.0.0-20190927101021-3ecffd272576
	github.com/jmoiron/sqlx v1.2.0
	github.com/lib/pq v1.2.0
	github.com/tantralabs/exchanges v0.0.0-20191106215748-4d3dd77e096e
	github.com/tantralabs/tradeapi v0.0.0-20191106232530-3eb5a2d402b7
	google.golang.org/api v0.13.0
)
