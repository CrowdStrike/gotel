package gotel

import (
	"code.google.com/p/gcfg"
)

type Config struct {
	Main struct {
		GotelOwnerEmail    string
		HoursBetweenAlerts int64
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

var (
	l *Logging
)

func init() {

}

func Conf(confPath string, sysLogEnabled bool) Config {
	config := Config{}
	l = &Logging{EnableSYSLOG: sysLogEnabled}
	l.setLogOutput()

	err := gcfg.ReadFileInto(&config, confPath)
	if err != nil {
		l.err("Conf error %q", err)
		panic("Unable to initialize configuration file properly")
	}
	return config
}
