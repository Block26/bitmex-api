module github.com/tantralabs/yantra

go 1.13

// replace github.com/tantralabs/eaopt => ../../tantralabs/eaopt
// replace github.com/tantralabs/tradeapi => ../../../github.com/tantralabs/tradeapi

require (
	firebase.google.com/go v3.10.0+incompatible
	github.com/aws/aws-sdk-go v1.25.28
	github.com/c-bata/goptuna v0.1.1-0.20191111040524-81338bb530f0
	github.com/chobie/go-gaussian v0.0.0-20150107165016-53c09d90eeaf
	github.com/d4l3k/talib v0.0.0-20180425021108-1b10e6a1ad95
	github.com/fatih/structs v1.1.0
	github.com/gocarina/gocsv v0.0.0-20190927101021-3ecffd272576
	github.com/google/uuid v1.1.1
	github.com/influxdata/influxdb-client-go v0.1.4
	github.com/influxdata/influxdb1-client v0.0.0-20190809212627-fc22c7df067e
	github.com/jmoiron/sqlx v1.2.0
	github.com/lib/pq v1.2.0
	github.com/tantralabs/eaopt v0.1.1-0.20191217192927-f9ded663d8fe // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gonum.org/v1/gonum v0.0.0-20190724133715-a8659125a966
	google.golang.org/api v0.13.0
	gopkg.in/src-d/go-git.v4 v4.8.1
)
