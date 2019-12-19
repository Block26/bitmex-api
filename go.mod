module github.com/tantralabs/TheAlgoV2

go 1.13

replace github.com/tantralabs/exchanges => ../../tantralabs/exchanges

replace github.com/tantralabs/tradeapi => ../../../github.com/tantralabs/tradeapi

require (
	firebase.google.com/go v3.10.0+incompatible
	github.com/MaxHalford/eaopt v0.1.1-0.20191017133541-37dd3a71cb48
	github.com/aws/aws-sdk-go v1.25.28
	github.com/c-bata/goptuna v0.1.0
	github.com/chobie/go-gaussian v0.0.0-20150107165016-53c09d90eeaf
	github.com/fatih/structs v1.1.0
	github.com/gocarina/gocsv v0.0.0-20190927101021-3ecffd272576
	github.com/google/uuid v1.1.1
	github.com/influxdata/influxdb1-client v0.0.0-20190809212627-fc22c7df067e
	github.com/jmoiron/sqlx v1.2.0
	github.com/lib/pq v1.2.0
	github.com/stretchr/testify v1.3.0 // indirect
	github.com/tantralabs/tradeapi v0.0.0-20191126194942-b5034f8f563e
	golang.org/x/net v0.0.0-20190918130420-a8b05e9114ab // indirect
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	gonum.org/v1/gonum v0.0.0-20190724133715-a8659125a966
	google.golang.org/api v0.13.0
	gopkg.in/src-d/go-git.v4 v4.8.1
)
