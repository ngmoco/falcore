package falcore

import (
	"errors"
	"log"
	"time"
)

// I really want to use log4go... but i need to support falling back to standard (shitty) logger :(
// I suggest using go-timber for the real logger
type Logger interface {
	// Matches the log4go interface
	Finest(arg0 interface{}, args ...interface{})
	Fine(arg0 interface{}, args ...interface{})
	Debug(arg0 interface{}, args ...interface{})
	Trace(arg0 interface{}, args ...interface{})
	Info(arg0 interface{}, args ...interface{})
	Warn(arg0 interface{}, args ...interface{}) error
	Error(arg0 interface{}, args ...interface{}) error
	Critical(arg0 interface{}, args ...interface{}) error
}

var logger Logger = NewStdLibLogger()

func SetLogger(newLogger Logger) {
	logger = newLogger
}

// Helper for calculating times
func TimeDiff(startTime time.Time, endTime time.Time) float32 {
	return float32(endTime.Sub(startTime)) / 1.0e9
}

// Global Logging
func Finest(arg0 interface{}, args ...interface{}) {
	logger.Finest(arg0, args...)
}

func Fine(arg0 interface{}, args ...interface{}) {
	logger.Fine(arg0, args...)
}

func Debug(arg0 interface{}, args ...interface{}) {
	logger.Debug(arg0, args...)
}

func Trace(arg0 interface{}, args ...interface{}) {
	logger.Trace(arg0, args...)
}

func Info(arg0 interface{}, args ...interface{}) {
	logger.Info(arg0, args...)
}

func Warn(arg0 interface{}, args ...interface{}) error {
	return logger.Warn(arg0, args...)
}

func Error(arg0 interface{}, args ...interface{}) error {
	return logger.Error(arg0, args...)
}

func Critical(arg0 interface{}, args ...interface{}) error {
	return logger.Critical(arg0, args...)
}

// This is a simple Logger implementation that
// uses the go log package for output.  It's not
// really meant for production use since it isn't
// very configurable.  It is a sane default alternative
// that allows us to not have any external dependencies.
// Use timber or log4go as a real alternative.
type StdLibLogger struct{}

func NewStdLibLogger() Logger {
	return new(StdLibLogger)
}

type level int

const (
	FINEST level = iota
	FINE
	DEBUG
	TRACE
	INFO
	WARNING
	ERROR
	CRITICAL
)

var (
	levelStrings = [...]string{"[FNST]", "[FINE]", "[DEBG]", "[TRAC]", "[INFO]", "[WARN]", "[EROR]", "[CRIT]"}
)

func (fl StdLibLogger) Finest(arg0 interface{}, args ...interface{}) {
	fl.Log(FINEST, arg0, args...)
}

func (fl StdLibLogger) Fine(arg0 interface{}, args ...interface{}) {
	fl.Log(FINE, arg0, args...)
}

func (fl StdLibLogger) Debug(arg0 interface{}, args ...interface{}) {
	fl.Log(DEBUG, arg0, args...)
}

func (fl StdLibLogger) Trace(arg0 interface{}, args ...interface{}) {
	fl.Log(TRACE, arg0, args...)
}

func (fl StdLibLogger) Info(arg0 interface{}, args ...interface{}) {
	fl.Log(INFO, arg0, args...)
}

func (fl StdLibLogger) Warn(arg0 interface{}, args ...interface{}) error {
	return fl.Log(WARNING, arg0, args...)
}

func (fl StdLibLogger) Error(arg0 interface{}, args ...interface{}) error {
	return fl.Log(ERROR, arg0, args...)
}

func (fl StdLibLogger) Critical(arg0 interface{}, args ...interface{}) error {
	return fl.Log(CRITICAL, arg0, args...)
}

func (fl StdLibLogger) Log(lvl level, arg0 interface{}, args ...interface{}) (e error) {
	defer func() {
		if x := recover(); x != nil {
			var ok bool
			if e, ok = x.(error); ok {
				return
			}
			e = errors.New("Um... barf")
		}
	}()
	switch first := arg0.(type) {
	case string:
		// Use the string as a format string
		argsNew := append([]interface{}{levelStrings[lvl]}, args...)
		log.Printf("%s "+first, argsNew...)
	case func() string:
		// Log the closure (no other arguments used)
		argsNew := append([]interface{}{levelStrings[lvl]}, first())
		log.Println(argsNew...)
	default:
		// Build a format string so that it will be similar to Sprint
		argsNew := append([]interface{}{levelStrings[lvl]}, args...)
		log.Println(argsNew...)
	}
	return nil
}
