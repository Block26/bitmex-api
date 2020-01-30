module github.com/tantralabs/yantra

go 1.13

//replace github.com/tantralabs/theo-engine/models => ../theo-engine/models

//replace github.com/tantralabs/theo-engine => ../theo-engine

//replace github.com/tantralabs/tradeapi => ../tradeapi

require (
	github.com/aws/aws-sdk-go v1.28.7
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/structs v1.1.0
	github.com/gocarina/gocsv v0.0.0-20191214001331-e6697589f2e0
	github.com/google/uuid v1.1.1
	github.com/influxdata/influxdb1-client v0.0.0-20191209144304-8bf82d3c094d
	github.com/jinzhu/copier v0.0.0-20190924061706-b57f9002281a
	github.com/jmoiron/sqlx v1.2.0
	github.com/kr/pretty v0.1.0 // indirect
	github.com/lib/pq v1.3.0
	github.com/markcheno/go-talib v0.0.0-20190307022042-cd53a9264d70
	github.com/tantralabs/eaopt v0.0.0-20200117031806-b5ae12d20441
	github.com/tantralabs/models v0.0.0-20200203185059-b6fff24390f2
	github.com/tantralabs/theo-engine v0.0.0-20200203203803-d79c0003cac2
	github.com/tantralabs/tradeapi v0.0.0-20200203203050-47b24237c8a1
	gonum.org/v1/gonum v0.6.2
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)
