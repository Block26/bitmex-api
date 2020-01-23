package logger

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
)

var displayLevel string = "info"
var level string = displayLevel
var zapLogger zap.SugaredLogger

func GetLevel() string {
	return level
}

func SetDisplayLevel(lvl string) {
	displayLevel = lvl
	Infof("Set logger display level to %v", displayLevel)
}

func SetLevel(lvl string) {
	if lvl == "" {
		level = "debug"
	} else {
		level = lvl
	}
	Debugf("Set logger level to %v", level)
}

func InitLogger(force bool) {
	if !force && zapLogger != nil {
		return
	}
	cfgString := fmt.Sprintf(`{
		"level": "%s",
		"encoding": "json",
		"outputPaths": ["stdout", "/tmp/logs", "yantra.log"],
		"errorOutputPaths": ["stderr"],
		"initialFields": {},
		"encoderConfig": {
		  "messageKey": "message",
		  "levelKey": "level",
		  "levelEncoder": "lowercase"
		}
	  }`, displayLevel)
	rawJSON := []byte(cfgString)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	rawLogger, err := cfg.Build()
	if err != nil {
		fmt.Printf("Error instantiating logger with config %v\n", cfgString)
	}
	zapLogger = rawLogger.Sugar()
	}

func Log(args ...interface{}) {
	if level == "error" {
		Error(args)
	} else if level == "debug" {
		Debug(args)
	} else {
		Info(args)
	}
}

func Debug(args ...interface{}) {
	InitLogger(false)
	zapLogger.Debug(args...)
}

func Info(args ...interface{}) {
	InitLogger(false)
	zapLogger.Info(args...)
}

func Error(args ...interface{}) {
	InitLogger(false)
	zapLogger.Error(args...)
}

func Logf(template string, args ...interface{}) {
	if level == "error" {
		Errorf(template, args...)
	} else if level == "debug" {
		Debugf(template, args...)
	} else {
		Infof(template, args...)
	}
}

func Debugf(template string, args ...interface{}) {
	InitLogger(false)
	zapLogger.Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	InitLogger(false)
	zapLogger.Infof(template, args...)
}

func Errorf(template string, args ...interface{}) {
	InitLogger(false)
	zapLogger.Errorf(template, args...)
}
