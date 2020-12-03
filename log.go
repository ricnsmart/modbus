package modbus

import (
	"log"
	"os"
)

type (
	// Logger interface allows implementations to provide to this package any
	// object that implements the methods defined in it.
	Logger interface {
		Info(v ...interface{})
		Warn(v ...interface{})
		Error(v ...interface{})
		Debug(v ...interface{})
	}

	DefaultLogger struct {
		logger *log.Logger
	}
)

func NewDefaultLogger() *DefaultLogger {
	logger := log.New(os.Stdout, "", 0)
	return &DefaultLogger{logger: logger}
}

func (d *DefaultLogger) Info(v ...interface{}) {
	v = append([]interface{}{"info"}, v...)
	d.logger.Println(v)
}

func (d *DefaultLogger) Warn(v ...interface{}) {
	v = append([]interface{}{"warn"}, v...)
	d.logger.Println(v)
}

func (d *DefaultLogger) Error(v ...interface{}) {
	v = append([]interface{}{"error"}, v...)
	d.logger.Println(v)
}

func (d *DefaultLogger) Debug(v ...interface{}) {
	v = append([]interface{}{"debug"}, v...)
	d.logger.Println(v)
}
