package main

import (
	"gopkg.in/gcfg.v1"
)

var l *logging

type config struct {
	Main struct {
		GotelOwnerEmail    string
		HoursBetweenAlerts int64
		DaysToStoreLogs    int
	}
	Smtp struct {
		Enabled     bool
		SmtpHost    string
		Fromaddress string
		ReplyTO     string
	}
	Pagerduty struct {
		Enabled    bool
		Servicekey string
	}
}

// NewConfig returns a gotel config with configPath and sysLogEnabled set.
// As part of initialization it will also parse the provided config file.
func NewConfig(confPath string, sysLogEnabled bool) config {
	conf := config{}
	l = &logging{EnableSYSLOG: sysLogEnabled}
	l.setLogOutput()

	err := gcfg.ReadFileInto(&conf, confPath)
	if err != nil {
		l.err("Conf error %q", err)
		panic("Unable to initialize configuration file properly")
	}
	return conf
}
