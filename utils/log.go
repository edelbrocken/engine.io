package utils

import (
	"engine.io/log"
)

var _log = log.NewLog("")

func Log() *log.Log {
	return _log
}
