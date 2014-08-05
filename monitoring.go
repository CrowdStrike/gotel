package gotel

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type Alerter interface {
	Alert(res Reservation)
	Name() string
	Bootstrap()
}

// holds info so we don't spam alerters every n seconds
var sentAlerts = make(map[string]time.Time)

// am I the leader?
var Coordinator bool = false

// stores a slice of alerter functions to call when we have an alert
var alertFuncs = []Alerter{}

var cfg Config

func Monitor(db *sql.DB) {
	if !Coordinator {
		Coordinator = isCoordinator(db)
	}
	printCoordinatorStatus()
	jobChecker(db)
}

func InitializeMonitoring(c Config) {
	cfg = c
	if cfg.Smtp.Enabled {
		smtp := new(SmtpAlerter)
		smtp.Cfg = c
		alertFuncs = append(alertFuncs, smtp)
	} else {
		l.info("SMTP Alerting disabled")

	}
	if cfg.Pagerduty.Enabled {
		pd := new(PagerDutyAlerter)
		pd.Cfg = c
		alertFuncs = append(alertFuncs, pd)
	} else {
		l.info("PagerDuty Alerting disabled")
	}

	for _, alerter := range alertFuncs {
		alerter.Bootstrap()
	}
}

func hasLock(db *sql.DB) bool {
	var lck int
	query := "SELECT GET_LOCK('gotel_lock', 3) as lck"
	rows, err := db.Query(query)
	if err != nil {
		l.warn("Unable to aquire lock\n")
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&lck)
		if err != nil {
			l.warn("Unable to aquire locking rows\n")
		}
		if lck == 1 {
			// holds a lock while the connection is alive
			l.info("Lock Aquired")
			return true
		} else {
			l.info("Unable to aquire coordinator lock. I must be a worker [%v]", lck)
		}
	}
	return false
}

func releaseLock(db *sql.DB) (bool, error) {
	releaseQuery := "SELECT RELEASE_LOCK('gotel_lock');"
	rows, err := db.Query(releaseQuery)
	if err != nil {
		l.warn("Unable to release lock\n")
		return false, errors.New("Unable to release lock")
	}
	defer rows.Close()
	return true, nil

}

// attempt to aquire coordinator lock to indicate this node should do the job checking
// in the future I'd like to have a zookeeper integration for a more "true" leader election scheme
// this is somewhat of a quickstart method so people don't have to also have ZK in their env
// split brain would be detected by the Coordinator not checking in so we'd be firing off an alert
func isCoordinator(db *sql.DB) bool {

	coordinatorNodeCnt := 0

	lockAquired := hasLock(db)
	if lockAquired {
		var (
			ip_address string
			node_id    int64
		)
		rows, err := db.Query("select ip_address, node_id from nodes")
		if err != nil {
			l.err("Unable to select nodes [%v]", err)
			return false
		}
		defer rows.Close()

		for rows.Next() {
			err := rows.Scan(&ip_address, &node_id)
			if err != nil {
				l.err("Unable to scan node rows [%v]", err)
				return false
			}

			// check to see if we have any other Coordinator nodes, or am i it?
			l.info("Checking ip [%s] for Coordinator status", ip_address)
			resp, err := http.Get(fmt.Sprintf("http://%s:8080/is-coordinator", ip_address))
			if err != nil {
				l.warn("Unable to contact node [%s] assuming offline", ip_address)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				l.warn("Didn't get a 200OK reply back from ip [%s]", ip_address)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				l.warn("Unable to read node response")
			}
			if string(body) == "true" {
				l.info("IP [%s] is reporting as a Coordinator", ip_address)
				coordinatorNodeCnt += 1
			}
			l.info("ip [%s] http Coordinator check returned [%s]", ip_address, body)

		}
		insertSelf(db)
		releaseLock(db)
		if coordinatorNodeCnt == 0 {
			// I'm the Coordinator!
			return true
		}
	}
	return false
}

func insertSelf(db *sql.DB) {
	ip, err := externalIP()
	if err != nil {
		l.err("Unable to get external IP [%v]", err)
	}
	rand.Seed(time.Now().UnixNano())
	seedID := rand.Intn(10000)

	stmt, err := db.Prepare("insert into nodes(ip_address, node_id) values(?, ?)")
	if err != nil {
		l.err("Unable to prepare insertself record %s", err)
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(ip, seedID)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			l.info("[%s] has already registered as a node", ip)
		} else {
			l.warn("Unable to insert insertself record %s", err)
		}
		return
	}
}

func printCoordinatorStatus() {
	if Coordinator {
		l.info("I am the coordinator node!\n")
	} else {
		l.info("I am the worker node!\n")
	}
}

// checks jobs and sends to workers to check on last update time
// we're not on the master we want to monitor the master to make sure it's running it's job checker
// mode will be master if the main jobs should run on this node
func jobChecker(db *sql.DB) {

	var query string
	if Coordinator {
		query = "select id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp from reservations"
	} else {
		// if we're a worker we just want to monitor the co-ordinator
		query = "select id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp from reservations where app='gotel' and component='coordinator'"
	}
	rows, err := db.Query(query)
	if err != nil {
		l.err("Unable to run job checker [%v]", err)
	}
	defer rows.Close()
	for rows.Next() {
		res := Reservation{}
		rows.Scan(&res.JobID, &res.App, &res.Component, &res.Owner, &res.Notify, &res.Frequency, &res.TimeUnits, &res.LastCheckin)

		if FailsSLA(res) && !alreadySentRecently(res) {
			alerterNames := []string{}
			for _, alerter := range alertFuncs {
				alerterNames = append(alerterNames, alerter.Name())
				go alerter.Alert(res)
			}
			storeAlert(res, db, alerterNames)
		}
	}
	storeJobRun(db)
}

// check to see if we've already sent this alert recently
func alreadySentRecently(res Reservation) bool {
	timeNow := time.Now().UTC()
	mapKey := res.App + res.Component
	var waitForNotifyTime time.Duration = time.Duration(cfg.Main.HoursBetweenAlerts) * time.Hour

	_, ok := sentAlerts[mapKey]
	if !ok {
		sentAlerts[mapKey] = timeNow
		l.info("Sending new alert for [%s/%s]", res.App, res.Component)
		return false
	} else {
		// check to see if the time elapsed goes over our threshold

		var duration time.Duration = timeNow.Sub(sentAlerts[mapKey])
		if duration >= waitForNotifyTime {
			sentAlerts[mapKey] = timeNow
			l.info("Sent alert already but duration ran out [%s/%s]", res.App, res.Component)
			return false
		}

	}
	l.info("Already sent alert for [%s/%s]", res.App, res.Component)
	return true

}

// this section will monitor the reservations and determine if any jobs haven't checked in within
//they're allotted timeframe
func FailsSLA(res Reservation) bool {
	l.info("Working on app [%s] component [%s]", res.App, res.Component)

	timeNow := time.Now().UTC()
	startTime := time.Unix(res.LastCheckin, 0)
	secondsAgo := int(timeNow.Sub(startTime).Seconds())
	if secondsAgo > getSecondsFromUnits(res.Frequency, res.TimeUnits) {
		// send job to alert on
		l.info("App Failed SLA [%s/%s] that is: %d seconds old\n", res.App, res.Component, secondsAgo)
		return true
	}
	return false
}

func storeAlert(res Reservation, db *sql.DB, alerters []string) {
	now := time.Now().UTC().Unix()
	altertNames := strings.Join(alerters, ",")
	stmt, err := db.Prepare("insert into alerts(app, component, alert_time, alerters) values(?, ?, ?, ?)")
	if err != nil {
		l.info("[ERROR] Unable to prepare storealert record %s", err)
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(res.App, res.Component, now, altertNames)
	if err != nil {
		l.info("[ERROR] Unable to insert alert record %s", err)
		return
	}
}

func storeJobRun(db *sql.DB) {
	mode := "worker"
	if Coordinator {
		mode = "coordinator"
	}
	now := time.Now().UTC().Unix()
	l.info("Storing job run, mode: [%s]\n", mode)
	storeCheckin(db, CheckIn{
		App:       "gotel",
		Component: mode,
	}, now)
}
