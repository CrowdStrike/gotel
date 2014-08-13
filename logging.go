package gotel

import (
	"fmt"
	"log"
	"log/syslog"
)

type logging struct {
	EnableSYSLOG bool
}

func (logger *logging) setLogOutput() {
	loggerName := "GOTEL"

	if logger.EnableSYSLOG {
		log.Println("SYSLOG logging enabled")
		syslogd, e := syslog.New(syslog.LOG_INFO, loggerName)
		if e == nil {
			log.SetOutput(syslogd)
		}
	} else {
		log.Println("SYSLOG logging NOT enabled")
	}
}

func (*logging) info(format string, a ...interface{}) {
	if len(a) < 1 {
		log.Printf("[INFO] %s\n", format)
	} else {
		msg := fmt.Sprintf(format, a...)
		log.Printf("[INFO] %s\n", msg)
	}

}

func (*logging) warn(format string, a ...interface{}) {
	if len(a) < 1 {
		log.Printf("[WARN] %s\n", format)
	} else {
		msg := fmt.Sprintf(format, a...)
		log.Printf("[WARN] %s\n", msg)
	}
}

func (*logging) err(format string, a ...interface{}) {
	if len(a) < 1 {
		log.Printf("[ERROR] %s\n", format)
	} else {
		msg := fmt.Sprintf(format, a...)
		log.Printf("[ERROR] %s\n", msg)
	}
}
