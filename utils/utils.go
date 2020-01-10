package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"encoding/base64"

	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/models"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

func getSecret(secretName string) string {
	// region := "us-west-1"

	//Create a Secrets Manager client
	svc := secretsmanager.New(session.New(), aws.NewConfig().WithRegion("us-west-1"))
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	// In this sample we only handle the specific exceptions for the 'GetSecretValue' API.
	// See https://docs.aws.amazon.com/secretsmanager/latest/apireference/API_GetSecretValue.html

	result, err := svc.GetSecretValue(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeDecryptionFailure:
				// Secrets Manager can't decrypt the protected secret text using the provided KMS key.
				fmt.Println(secretsmanager.ErrCodeDecryptionFailure, aerr.Error())

			case secretsmanager.ErrCodeInternalServiceError:
				// An error occurred on the server side.
				fmt.Println(secretsmanager.ErrCodeInternalServiceError, aerr.Error())

			case secretsmanager.ErrCodeInvalidParameterException:
				// You provided an invalid value for a parameter.
				fmt.Println(secretsmanager.ErrCodeInvalidParameterException, aerr.Error())

			case secretsmanager.ErrCodeInvalidRequestException:
				// You provided a parameter value that is not valid for the current state of the resource.
				fmt.Println(secretsmanager.ErrCodeInvalidRequestException, aerr.Error())

			case secretsmanager.ErrCodeResourceNotFoundException:
				// We can't find the resource that you asked for.
				fmt.Println(secretsmanager.ErrCodeResourceNotFoundException, aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		fmt.Println(err.Error())
		return "error"
	}

	// Decrypts secret using the associated KMS CMK.
	// Depending on whether the secret is a string or binary, one of these fields will be populated.
	var secretString, decodedBinarySecret string
	if result.SecretString != nil {
		secretString = *result.SecretString
		return secretString
	} else {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(result.SecretBinary)))
		len, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, result.SecretBinary)
		if err != nil {
			fmt.Println("Base64 Decode Error:", err)
			return "error"
		}
		decodedBinarySecret = string(decodedBinarySecretBytes[:len])
		return decodedBinarySecret
	}
}

// LoadConfiguration Load a config file containing sensitive information from a local
// json file or from an amazon secrets file
func LoadConfiguration(file string, secret bool) models.Config {
	var config models.Config
	if secret {
		secret := getSecret(file)
		config = models.Config{}
		json.Unmarshal([]byte(secret), &config)
		return config
	} else {
		fmt.Printf("Loading config from file: %v\n", file)
		configFile, err := os.Open(file)
		defer configFile.Close()
		if err != nil {
			log.Println(err.Error())
		}
		jsonParser := json.NewDecoder(configFile)
		jsonParser.Decode(&config)
		fmt.Printf("Parsed json: %v\n", jsonParser)
		return config
	}
}

func ConvertTradeBinsToBars(bins []iex.TradeBin) (bars []*models.Bar) {
	bars = make([]*models.Bar, len(bins))
	for i, _ := range bins {
		bar := ConvertTradeBinToBar(bins[i])
		bars[i] = &bar
	}
	return
}

func ConvertTradeBinToBar(bin iex.TradeBin) models.Bar {
	return models.Bar{
		Timestamp: bin.Timestamp.Unix() * 1000,
		Open:      bin.Open,
		High:      bin.High,
		Low:       bin.Low,
		Close:     bin.Close,
	}
}

// ConstrainFloat Limit a float to min, max, and decimal places
func ConstrainFloat(x float64, min float64, max float64, decimals int) float64 {
	return ToFixed(math.Max(min, math.Min(x, max)), decimals)
}

// GetOHLCBars Break down the bars into open, high, low, close arrays that are easier to manipulate.
func GetOHLCBars(bars []*models.Bar) ([]float64, []float64, []float64, []float64) {
	open := make([]float64, len(bars))
	high := make([]float64, len(bars))
	low := make([]float64, len(bars))
	close := make([]float64, len(bars))
	for i := range bars {
		open[i] = bars[i].Open
		high[i] = bars[i].High
		low[i] = bars[i].Low
		close[i] = bars[i].Close
	}
	return open, high, low, close
}

func ToIntTimestamp(timeString string) int {
	layout := "2006-01-02 15:04:05"
	// if strings.Contains(timeString, "+0000 UTC") {
	// 	timeString = strings.Replace(timeString, "+0000 UTC", "", 1)
	// }
	// timeString = strings.TrimSpace(timeString)
	timeString = timeString[:19]
	currentTime, err := time.Parse(layout, timeString)
	if err != nil {
		fmt.Printf("Error parsing timeString: %v\n", err)
	}
	return int(currentTime.UnixNano() / int64(time.Millisecond))
}

func ToTimeObject(timeString string) time.Time {
	layout := "2006-01-02 15:04:05"
	// if strings.Contains(timeString, "+0000 UTC") {
	// 	timeString = strings.Replace(timeString, "+0000 UTC", "", 1)
	// }
	// timeString = strings.TrimSpace(timeString)
	timeString = timeString[:19]
	currentTime, err := time.Parse(layout, timeString)
	if err != nil {
		fmt.Printf("Error parsing timeString: %v", err)
	}
	return currentTime
}

func TimestampToTime(timestamp int) time.Time {
	timeInt, err := strconv.ParseInt(strconv.Itoa(timestamp/1000), 10, 64)
	if err != nil {
		panic(err)
	}
	return time.Unix(timeInt, 0)
}

func TimeToTimestamp(timeObject time.Time) int {
	return int(timeObject.UnixNano() / 1000000)
}

// Round round a number to a decimal place
func Round(x, decimal float64) float64 {
	return math.Round(x/decimal) * decimal
}

func ReverseArr(a []float64) []float64 {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func reverseBars(a []*models.Bar) []*models.Bar {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func Arange(min float64, max float64, step float64) []float64 {
	a := make([]float64, int32((max-min)/step)+1)
	for i := range a {
		a[i] = float64(min+step) + (float64(i) * step)
	}
	return a
}

func CalculateDifference(x float64, y float64) float64 {
	//Get percentage difference between 2 numbers
	if y == 0 {
		y = 1
	}
	return (x - y) / y
}

// ExpArr Apply an exponent to a slice
func ExpArr(arr []float64, exp float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		if arr[i] > 1 {
			a[i] = exponent(arr[i]-1, exp)
		} else {
			a[i] = exponent(arr[i], exp) - 1
		}
	}
	return a
}

// MulArrs Multiply a slice by another slice of the same length
func MulArrs(a []float64, b []float64) []float64 {
	n := make([]float64, len(a))
	for i := range a {
		n[i] = a[i] * b[i]
	}
	return n
}

// MulArr Multiply a slice by a float
func MulArr(arr []float64, multiple float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) * multiple
	}
	return a
}

// DivArr Divide all elements of a slice by a float
func DivArr(arr []float64, divisor float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) / divisor
	}
	return a
}

// SumArr Get the sum of all elements in a slice
func SumArr(arr []float64) float64 {
	sum := 0.0
	for i := range arr {
		sum = sum + arr[i]
	}
	return sum
}

func exponent(x, y float64) float64 {
	return math.Pow(x, y)
}

// CreateKeyValuePairs make a string interface human readable
func CreateKeyValuePairs(m map[string]interface{}) string {
	b := new(bytes.Buffer)
	fmt.Fprint(b, "\n{\n")
	for key, value := range m {
		fmt.Fprint(b, " ", key, ": ", value, ",\n")
	}
	fmt.Fprint(b, "}\n")
	return b.String()
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func ToFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func RoundToNearest(num float64, interval float64) float64 {
	return math.Round(num/interval) * interval
}

func AdjustForSlippage(premium float64, side string, slippage float64) float64 {
	adjPremium := premium
	fmt.Printf("Premium %v, with slippage %v\n", premium, premium*(1.-slippage))
	if side == "buy" {
		adjPremium = premium * (1. + slippage)
	} else if side == "sell" {
		adjPremium = premium * (1. - slippage)
	}
	return adjPremium
}

func GetDeribitOptionSymbol(expiry int, strike float64, currency string, optionType string) string {
	expiryTime := time.Unix(int64(expiry/1000), 0)
	year := strconv.Itoa(expiryTime.Year())[2:3]
	month := strings.ToUpper(expiryTime.Month().String())[:3]
	day := strconv.Itoa(expiryTime.Day())
	return "BTC-" + strconv.Itoa(int(strike)) + "-" + day + month + year + "-" + strings.ToUpper(optionType)
}

func GetNextFriday(currentTime time.Time) time.Time {
	dayDiff := currentTime.Weekday()
	if dayDiff <= 0 {
		dayDiff += 7
	}
	return currentTime.Truncate(24 * time.Hour).Add(time.Hour * time.Duration(24*dayDiff))
}

func GetLastFridayOfMonth(currentTime time.Time) time.Time {
	year, month, _ := currentTime.Date()
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1).Day()
	currentTime = time.Date(year, month, lastOfMonth, 0, 0, 0, 0, time.UTC)
	for i := lastOfMonth; i > 0; i-- {
		if currentTime.Weekday() == 5 {
			return currentTime
		}
		currentTime = currentTime.Add(-time.Hour * time.Duration(24))
	}
	return currentTime
}

func GetQuarterlyExpiry(currentTime time.Time, minDays int) time.Time {
	year, month, _ := currentTime.Add(time.Hour * time.Duration(24*minDays)).Date()
	// Get nearest quarterly month
	quarterlyMonth := month + (month % 4)
	if quarterlyMonth >= 12 {
		year += 1
		quarterlyMonth = quarterlyMonth % 12
	}
	lastFriday := GetLastFridayOfMonth(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC))
	// fmt.Printf("Got quarterly expiry %v\n", lastFriday)
	return lastFriday
}

func IntInSlice(a int, list []int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}