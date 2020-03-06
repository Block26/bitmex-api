package tests

import (
	"testing"
	"time"

	"github.com/tantralabs/database"
	"github.com/tantralabs/yantra/models"
)

func SetupDataModel() models.Data {
	start := time.Date(2019, 11, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	data := database.GetData("XBTUSD", "bitmex", "1m", start, end)
	return models.SetupDataModel(data)
}

func TestSetupDataModel(t *testing.T) {
	start := time.Date(2019, 11, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	data := models.SetupDataModel(database.GetData("XBTUSD", "bitmex", "1m", start, end))
	length := len(data.GetBarData())
	if length != 43200 {
		t.Error(length, "is not 43200")
	}

	// start 1 minut later
	start = time.Date(2019, 11, 01, 0, 01, 0, 0, time.UTC)
	data = models.SetupDataModel(database.GetData("XBTUSD", "bitmex", "1m", start, end))
	length = len(data.GetBarData())
	if length != 43140 {
		t.Error(length, "is not 43140")
	}
}

func TestGetHourData(t *testing.T) {
	data := SetupDataModel()
	firstLength := len(data.GetHourData().Timestamp)
	secondLength := len(data.GetHourData().Timestamp)

	if firstLength != secondLength {
		t.Error("The dataset has changed and it should be the same.")
	}

	lastHourBarTS := data.GetHourData().Timestamp[len(data.GetHourData().Timestamp)-1]
	lastMinuteBarTS := data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastHourBarTS != lastMinuteBarTS {
		t.Error("The both data sets should end on the same timestamp", lastHourBarTS, "!=", lastMinuteBarTS)
	}

	// get more data
	start := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 02, 0, 0, 0, 0, time.UTC)
	latestData := database.GetData("XBTUSD", "bitmex", "1m", start, end)

	data.AddData(latestData)

	thirdLength := len(data.GetHourData().Timestamp)

	if thirdLength == secondLength {
		t.Error("The dataset has not changed and it should be longer.")
	}

	lastHourBarTS = data.GetHourData().Timestamp[len(data.GetHourData().Timestamp)-1]
	lastMinuteBarTS = data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastHourBarTS != lastMinuteBarTS {
		t.Error("The both data sets should end on the same timestamp", lastHourBarTS, "!=", lastMinuteBarTS)
	}

	// add the same data
	data.AddData(latestData)

	fourthLength := len(data.GetHourData().Timestamp)

	if thirdLength != fourthLength {
		t.Error("The dataset has changed and it should be the same.")
	}

	lastHourBarTS = data.GetHourData().Timestamp[len(data.GetHourData().Timestamp)-1]
	lastMinuteBarTS = data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastHourBarTS != lastMinuteBarTS {
		t.Error("The both data sets should end on the same timestamp", lastHourBarTS, "!=", lastMinuteBarTS)
	}
}

func TestGetMinuteData(t *testing.T) {
	data := SetupDataModel()
	firstLength := len(data.GetMinuteData().Timestamp)
	secondLength := len(data.GetMinuteData().Timestamp)

	if firstLength != secondLength {
		t.Error("The dataset has changed and it should be the same.")
	}

	lastMinuteBarTS := data.GetBarData()[len(data.GetBarData())-1].Timestamp
	lastMinuteOHLCVTS := data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastMinuteBarTS != lastMinuteOHLCVTS {
		t.Error("The both data sets should end on the same timestamp", lastMinuteBarTS, "!=", lastMinuteOHLCVTS)
	}

	// get more data
	start := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 02, 0, 35, 0, 0, time.UTC)
	latestData := database.GetData("XBTUSD", "bitmex", "1m", start, end)

	data.AddData(latestData)

	thirdLength := len(data.GetMinuteData().Timestamp)

	if thirdLength == secondLength {
		t.Error("The dataset has not changed and it should be longer.")
	}

	lastMinuteBarTS = data.GetBarData()[len(data.GetBarData())-1].Timestamp
	lastMinuteOHLCVTS = data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastMinuteBarTS != lastMinuteOHLCVTS {
		t.Error("The both data sets should end on the same timestamp", lastMinuteBarTS, "!=", lastMinuteOHLCVTS)
	}

	// add the same data
	data.AddData(latestData)

	fourthLength := len(data.GetMinuteData().Timestamp)

	if thirdLength != fourthLength {
		t.Error("The dataset has changed and it should be the same.")
	}

	lastMinuteBarTS = data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]
	lastMinuteOHLCVTS = data.GetMinuteData().Timestamp[len(data.GetMinuteData().Timestamp)-1]

	if lastMinuteBarTS != lastMinuteOHLCVTS {
		t.Error("The both data sets should end on the same timestamp", lastMinuteBarTS, "!=", lastMinuteOHLCVTS)
	}

}

func TestBarData(t *testing.T) {
	data := SetupDataModel()
	firstLength := len(data.GetBarData())
	secondLength := len(data.GetBarData())

	if firstLength != secondLength {
		t.Error("The dataset has changed and it should be the same.")
	}

	// get more data
	start := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 02, 0, 0, 0, 0, time.UTC)
	latestData := database.GetData("XBTUSD", "bitmex", "1m", start, end)

	data.AddData(latestData)

	thirdLength := len(data.GetBarData())

	if thirdLength == secondLength {
		t.Error("The dataset has not changed and it should be longer.")
	}

	// add the same data
	data.AddData(latestData)

	fourthLength := len(data.GetBarData())

	if thirdLength != fourthLength {
		t.Error("The dataset has changed and it should be the same. Old length", thirdLength, "New Length", fourthLength)
	}
}
