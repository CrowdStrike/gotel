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

type alerter interface {
	Alert(res reservation) bool
	Name() string
	Bootstrap()
}

var (
	// holds info so we don't spam alerters every n seconds
	sentAlerts = make(map[string]time.Time)
	// this designates the instance as the coordinating instanceS
	coordinator = false
	// stores a slice of alerter functions to call when we have an alert
	alertFuncs = []alerter{}
	cfg        config
	// our current IP address
	myIP string
)

func init() {
	var err error
	myIP, err = externalIP()
	if err != nil {
		myIP = "N/A"
		l.err("Unable to aquire own IP Address")
	}
}

// Monitor checks existing reservations for late arrivals
func Monitor(db *sql.DB) {
	if !coordinator {
		coordinator = isCoordinator(db)
	}
	printCoordinatorStatus()
	jobChecker(db)
}

// InitializeMonitoring sets up alerters based on configuration
func InitializeMonitoring(c config, db *sql.DB) {
	cfg = c
	if cfg.Smtp.Enabled {
		smtp := new(smtpAlerter)
		smtp.Cfg = c
		alertFuncs = append(alertFuncs, smtp)
	} else {
		l.info("SMTP Alerting disabled")
	}
	if cfg.Pagerduty.Enabled {
		pd := new(pagerDutyAlerter)
		pd.Cfg = c
		alertFuncs = append(alertFuncs, pd)
	} else {
		l.info("PagerDuty Alerting disabled")
	}

	for _, alerter := range alertFuncs {
		alerter.Bootstrap()
	}

	// set up a ticker that runs every day that checks to clean up old logs to preserve disk space

	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for t := range ticker.C {
			if coordinator {
				l.info("Running log cleanup at [%v]", t)
				cleanUp(db, c.Main.DaysToStoreLogs)
			}
		}
	}()

}

//--------------------- PRIVATE FUNCS ------------------------------

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
		}
		l.info("Unable to aquire coordinator lock. I must be a worker [%v]", lck)

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
// split brain would be detected by the coordinator not checking in so we'd be firing off an alert
func isCoordinator(db *sql.DB) bool {

	coordinatorNodeCnt := 0

	lockAquired := hasLock(db)
	if lockAquired {
		var (
			ipAddress string
			nodeID    int64
		)
		rows, err := db.Query("select ip_address, node_id from nodes")
		if err != nil {
			l.err("Unable to select nodes [%v]", err)
			return false
		}
		defer rows.Close()

		for rows.Next() {
			err := rows.Scan(&ipAddress, &nodeID)
			if err != nil {
				l.err("Unable to scan node rows [%v]", err)
				return false
			}
			if ipAddress == myIP {
				continue
			}
			// check to see if we have any other coordinator nodes, or am i it?
			l.info("Checking ip [%s] for coordinator status", ipAddress)
			resp, err := http.Get(fmt.Sprintf("http://%s:8080/is-coordinator", ipAddress))
			if err != nil {
				l.warn("Unable to contact node [%s] assuming offline", ipAddress)
				removeNode(db, ipAddress)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				l.warn("Didn't get a 200OK reply back from ip [%s]", ipAddress)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				l.warn("Unable to read node response")
			}
			if string(body) == "true" {
				l.info("IP [%s] is reporting as a coordinator", ipAddress)
				coordinatorNodeCnt++
			}
			l.info("ip [%s] http coordinator check returned [%s]", ipAddress, body)

		}
		insertSelf(db)
		releaseLock(db)
		if coordinatorNodeCnt == 0 {
			// I'm the coordinator!
			return true
		}
	}
	return false
}

func removeNode(db *sql.DB, ipAddress string) {
	external, err := externalIP()
	if external == ipAddress {
		return
	}
	if err != nil {
		l.warn("Unable to delete offline node [%v]", err)
	}
	stmt, err := db.Prepare("DELETE FROM nodes WHERE ip_address=?")
	if err != nil {
		l.warn("Unable to prepare record %s", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(ipAddress)
	if err != nil {
		l.warn("Unable to run delete operation for remove node [%v]", err)
	}
	l.info("Node [%s] was removed from DB", ipAddress)
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
	if coordinator {
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
	if coordinator {
		query = "select id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp from reservations"
	} else {
		// if we're a worker we just want to monitor the co-ordinator
		query = "select id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp from reservations where app='gotel' and component='coordinator'"
	}
	rows, err := db.Query(query)
	defer rows.Close()
	if err != nil {
		l.err("Unable to run job checker [%v]", err)
		return
	}

	for rows.Next() {
		res := reservation{}
		rows.Scan(&res.JobID, &res.App, &res.Component, &res.Owner, &res.Notify, &res.Frequency, &res.TimeUnits, &res.LastCheckin)

		if FailsSLA(res) {
			alerterNames := []string{}
			for _, alerter := range alertFuncs {
				alerterNames = append(alerterNames, alerter.Name())

				if !alreadySentRecently(res, alerter.Name()) {
					if alerter.Alert(res) {
						updateSentRecently(res, alerter.Name())
						storeAlert(res, db, alerterNames)
					}
				} else {
					l.info("Already sent alert for [%s/%s/%s]", res.App, res.Component, alerter.Name())
				}
			}
		}
	}
	storeJobRun(db)
}

func (r *reservation) mapKey(alerterName string) string {
	return r.App + r.Component + alerterName
}

// check to see if we've already sent this alert recently
func alreadySentRecently(res reservation, alerterName string) bool {
	timeNow := time.Now().UTC()
	waitForNotifyTime := time.Duration(cfg.Main.HoursBetweenAlerts) * time.Hour

	lastSent, ok := sentAlerts[res.mapKey(alerterName)]
	if !ok {
		return false
	}

	// check to see if the time elapsed goes over our threshold
	duration := timeNow.Sub(lastSent)
	if duration >= waitForNotifyTime {
		return false
	}

	return true
}

func updateSentRecently(res reservation, alerterName string) {
	sentAlerts[res.mapKey(alerterName)] = time.Now()
}

// FailsSLA monitors the reservations and determines if any jobs haven't checked in within
// their allotted timeframe
func FailsSLA(res reservation) bool {
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

func storeAlert(res reservation, db *sql.DB, alerters []string) {
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
	if coordinator {
		mode = "coordinator"
	}
	now := time.Now().UTC().Unix()
	l.info("Storing job run, mode: [%s]\n", mode)
	storeCheckin(db, checkin{
		App:       "gotel",
		Component: mode,
	}, now)
}

// Cleanup should run on a scheduled ticker to allow GoTel to clean up after itself to prevent disk space issues in the
// DB as the process is meant to run for years.
func cleanUp(db *sql.DB, daysToStoreLogs int) {

	// grab the unix time that was daysToStoreLogs ago, cleanup anything older than that to keep db size down
	timeNow := time.Now().UTC().AddDate(0, 0, -daysToStoreLogs).Unix()

	// clean up housekeeping
	stmt, err := db.Prepare("DELETE FROM housekeeping WHERE last_checkin_timestamp < ?")
	if err != nil {
		l.err("Unable to prepare cleaup housekeeping statement")
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(timeNow)
	if err != nil {
		l.err("Unable cleanup old housekeeping logs, this could be bad [%v]", err)
		return
	}

	// clean up alerts
	stmt, err = db.Prepare("DELETE FROM alerts WHERE alert_time < ?")
	if err != nil {
		l.err("Unable to prepare cleaup alerts statement")
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(timeNow)
	if err != nil {
		l.err("Unable cleanup old alerts logs, this could be bad [%v]", err)
		return
	}

}
