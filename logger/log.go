package logger

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
)

var level string = "debug"
var logger *zap.SugaredLogger

func SetLevel(lvl string) {
	level = lvl
	InitLogger(true)
	Infof("Set logger to level %v", level)
}

func InitLogger(force bool) {
	if !force && logger != nil {
		return
	}
	cfgString := fmt.Sprintf(`{
		"level": "%s",
		"encoding": "json",
		"outputPaths": ["stdout", "/tmp/logs"],
		"errorOutputPaths": ["stderr"],
		"initialFields": {},
		"encoderConfig": {
		  "messageKey": "message",
		  "levelKey": "level",
		  "levelEncoder": "lowercase"
		}
	  }`, level)
	rawJSON := []byte(cfgString)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	rawLogger, err := cfg.Build()
	if err != nil {
		fmt.Printf("Error instantiating logger with config %v\n", cfgString)
	}
	logger = rawLogger.Sugar()
	logger.Infof("Initialized logger with config %v", cfgString)
}

func Debug(args ...interface{}) {
	InitLogger(false)
	logger.Debug(args)
}

func Info(args ...interface{}) {
	InitLogger(false)
	logger.Info(args)
}

func Error(args ...interface{}) {
	InitLogger(false)
	logger.Error(args)
}

func Debugf(template string, args ...interface{}) {
	InitLogger(false)
	logger.Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	InitLogger(false)
	logger.Infof(template, args...)
}

func Errorf(template string, args ...interface{}) {
	InitLogger(false)
	logger.Errorf(template, args...)
}
