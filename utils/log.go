package utils

import (
	"github.com/edelbrocken/engine.io/log"
)

var _log = log.NewLog("")

func Log() *log.Log {
	return _log
}
