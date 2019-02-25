package gotel

import (
	"gopkg.in/gcfg.v1"
)

var l *logging

// Config is the service configuration
type Config struct {
	Main struct {
		GotelOwnerEmail    string
		HoursBetweenAlerts int64
		DaysToStoreLogs    int
	}
	SMTP struct {
		Enabled     bool
		FromAddress string
		ReplyTO     string
	}
	PagerDuty struct {
		Enabled    bool
		ServiceKey string
	}
}

// NewConfig returns a gotel config with configPath and sysLogEnabled set.
// As part of initialization it will also parse the provided config file.
func NewConfig(confPath string, sysLogEnabled bool) Config {
	conf := Config{}
	l = &logging{EnableSYSLOG: sysLogEnabled}
	l.setLogOutput()

	err := gcfg.ReadFileInto(&conf, confPath)
	if err != nil {
		l.err("Conf error %q", err)
		panic("Unable to initialize configuration file properly")
	}
	return conf
}
