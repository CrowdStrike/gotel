package gotel

import (
	"flag"
	"github.com/stvp/pager"
)

var (
	pdServiceKey string
)

func init() {
	flag.StringVar(&pdServiceKey, "GOTEL_PD_SERVICE_KEY", "", "PagerDuty service key to use for alerts")
}

type pagerDutyAlerter struct {
	Cfg Config
}

func (s *pagerDutyAlerter) Bootstrap() {
	if pdServiceKey != "" {
		s.Cfg.PagerDuty.ServiceKey = pdServiceKey
	}
}

func (s *pagerDutyAlerter) Name() string {
	return "PagerDuty"
}

func (s *pagerDutyAlerter) Alert(res reservation) bool {

	l.info("PagerDuty API key [%s]", s.Cfg.PagerDuty.ServiceKey)

	pager.ServiceKey = s.Cfg.PagerDuty.ServiceKey
	incidentKey, err := pager.Trigger(res.AlertMessage)

	if err != nil {
		l.err("[ERROR] Unable to create PagerDuty alert for job [%s] component [%s] error [%v]\n", res.App,
			res.Component, err)
		return false
	}
	l.info("PagerDuty incident key created %s\n", incidentKey)
	return true
}
