package logger

import (
	"fmt"
)

var logLevel int = 0

type logLevelOpts struct {
	Error int
	Live  int
	Debug int
	Info  int
}

// FillType set the base definitions for the supported backtest fill types
func LogLevel() logLevelOpts {
	r := logLevelOpts{}
	r.Error = -1
	r.Live = 0
	r.Info = 1
	r.Debug = 2
	return r
}

func SetLogLevel(l int) {
	logLevel = l
}

func Error(args ...interface{}) {
	if logLevel >= LogLevel().Error {
		fmt.Println(args...)
	}
}

func Debug(args ...interface{}) {
	if logLevel >= LogLevel().Debug {
		fmt.Println(args...)
	}
}

func Info(args ...interface{}) {
	if logLevel >= LogLevel().Info {
		fmt.Println(args...)
	}
}

func Live(args ...interface{}) {
	if logLevel >= LogLevel().Live {
		fmt.Println(args...)
	}
}

func Debugf(template string, args ...interface{}) {
	if logLevel >= LogLevel().Debug {
		fmt.Printf(template, args...)
	}
}

func Infof(template string, args ...interface{}) {
	if logLevel >= LogLevel().Info {
		fmt.Printf(template, args...)
	}
}

func Errorf(template string, args ...interface{}) {
	if logLevel >= LogLevel().Error {
		fmt.Printf(template, args...)
	}
}

func Livef(template string, args ...interface{}) {
	if logLevel >= LogLevel().Live {
		fmt.Printf(template, args...)
	}
}
