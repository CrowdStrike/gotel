package gotel

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Endpoint holds the reference to our DB connection
type Endpoint struct {
	Db *sql.DB
}

// reservation is when an app first registers that it will be checking into our gotel
type reservation struct {
	JobID       int    `json:"job_id"`
	Owner       string `json:"owner"`
	Notify      string `json:"notify"`
	App         string `json:"app"`
	Component   string `json:"component"`
	Frequency   int    `json:"frequency"`
	TimeUnits   string `json:"time_units"`
	LastCheckin int64  `json:"last_checkin"`
}

// checkin holds a struct that is populated when an app checks in as still alive
type checkin struct {
	App       string `json:"app"`
	Component string `json:"component"`
	Notes     string `json:"notes"`
}

// checkOut is for removing reservations
type checkOut struct {
	App       string `json:"app"`
	Component string `json:"component"`
}

// snooze holds a struct for when users want to pause an alert for maintenance
type snooze struct {
	App       string `json:"app"`
	Component string `json:"component"`
	Duration  int    `json:"duration"`
	TimeUnits string `json:"time_units"`
}

// InitDb initialzes and then bootstraps the database
func InitDb(host, user, pass string, conf config) *sql.DB {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:3306)/gotel", user, pass, host))
	if err != nil {
		panic(err)
	}
	err = db.Ping()
	if err != nil {
		panic(fmt.Sprintf("Unable to ping the DB at host [%s] user [%s]", host, user))
	}
	bootstrapDb(db, conf)
	return db
}

func storeReservation(db *sql.DB, r *reservation) (bool, error) {

	// get current unix time one day into the future as the initial insert data, give it one day to bake
	tomorrow := time.Now().Add(24 * time.Hour).UTC().Unix()
	now := time.Now().UTC().Unix()

	seconds := getSecondsFromUnits(r.Frequency, r.TimeUnits)
	if seconds < 10 {
		return false, errors.New("Unable to store reservations for less than 10 seconds at this time, for no real reason.")
	}

	stmt, err := db.Prepare(`INSERT INTO reservations(app, component, owner, notify, frequency, time_units, inserted_timestamp, last_checkin_timestamp) 
		VALUES(?,?,?,?,?,?,?,?) 
		ON DUPLICATE KEY UPDATE notify=?, frequency=?, time_units=?
		`)

	if err != nil {
		l.warn("unable to prepare statement %s", err)
		return false, errors.New("Unable to save record")
	}
	defer stmt.Close()

	res, err := stmt.Exec(r.App, r.Component, r.Owner, r.Notify, r.Frequency, r.TimeUnits, now, tomorrow, r.Notify, r.Frequency, r.TimeUnits)
	if err != nil {
		l.warn("Unable to insert record %s", err)
		return false, errors.New("Unable to save record")
	}

	rowCnt, err := res.RowsAffected()
	if err != nil {
		return false, errors.New("Unable to save record")
	}

	l.info("Insertedaffected = %d\n", rowCnt)
	return true, nil

}

func logHouseKeeping(db *sql.DB, c checkin, now int64) (bool, error) {

	//Insert
	stmt, err := db.Prepare("insert into housekeeping(app, component, notes, last_checkin_timestamp) values(?, ?, ?, ?)")
	if err != nil {
		l.warn("Unable to prepare record %s", err)
		return false, errors.New("Unable to store checkin")
	}
	defer stmt.Close()
	_, err = stmt.Exec(c.App, c.Component, c.Notes, now)
	if err != nil {
		l.warn("Unable to insert record %s", err)
		return false, errors.New("Unable to store checkin")
	}

	return true, nil
}

func storeCheckin(db *sql.DB, c checkin, now int64) (bool, error) {

	stmt, err := db.Prepare("UPDATE reservations SET last_checkin_timestamp = ?, num_checkins = num_checkins + 1 WHERE app=? AND component=?")
	if err != nil {
		l.warn("Unable to prepare record %s", err)
		return false, errors.New("Unable to prepare checkin")
	}
	defer stmt.Close()
	_, err = stmt.Exec(now, c.App, c.Component)
	if err != nil {
		l.warn("Unable to update reservation %s", err)
		return false, errors.New("Unable to store checkin")
	}

	return true, nil
}

func storeCheckOut(db *sql.DB, c *checkOut) (bool, error) {

	stmt, err := db.Prepare("DELETE FROM reservations WHERE app=? AND component=?")
	if err != nil {
		l.warn("Unable to prepare record %s", err)
		return false, errors.New("Unable to prepare checkin")
	}
	defer stmt.Close()
	_, err = stmt.Exec(c.App, c.Component)
	if err != nil {
		l.warn("Unable to update reservation %s", err)
		return false, errors.New("Unable to store checkin")
	}
	return true, nil
}

func storeSnooze(db *sql.DB, p *snooze) (bool, error) {
	futureSeconds := getSecondsFromUnits(p.Duration, p.TimeUnits)

	pausedTime := time.Now().Add(time.Duration(futureSeconds) * time.Second).UTC().Unix()

	stmt, err := db.Prepare("UPDATE reservations SET last_checkin_timestamp = ? WHERE app=? AND component=?")
	if err != nil {
		l.warn("Unable to prepare record %s", err)
		return false, errors.New("Unable to prepare snooze")
	}
	defer stmt.Close()

	_, err = stmt.Exec(pausedTime, p.App, p.Component)
	if err != nil {
		l.warn("Unable to update snooze %s", err)
		return false, errors.New("Unable to store snooze")
	}

	return true, nil
}

func getSecondsFromUnits(freq int, units string) int {
	var seconds int
	if units == "seconds" {
		seconds = freq
	} else if units == "minutes" {
		seconds = 60 * freq
	} else if units == "hours" {
		seconds = 60 * 60 * freq
	} else {
		seconds = 60 * 60 * 24 * freq
	}
	return seconds
}

func bootstrapDb(db *sql.DB, conf config) {

	l.info("Bootstrapping GoTel DB tables")

	alertSQL := `
			CREATE TABLE IF NOT EXISTS  alerts (
			  id int(11) unsigned NOT NULL AUTO_INCREMENT,
			  app varchar(30) DEFAULT NULL,
			  component varchar(30) DEFAULT NULL,
			  alert_time int(11) DEFAULT NULL,
			  alerters text DEFAULT NULL,
			  insert_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			  PRIMARY KEY (id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8;`

	_, err := db.Exec(alertSQL)
	if err != nil {
		panic(err)
		return
	}

	sql := `
		CREATE TABLE IF NOT EXISTS reservations (
		  id int(11) NOT NULL AUTO_INCREMENT,
		  app varchar(30) DEFAULT NULL,
		  component varchar(30) DEFAULT NULL,
		  owner varchar(30) DEFAULT NULL,
		  notify varchar(30) DEFAULT NULL,
		  frequency int(11) DEFAULT NULL,
		  time_units varchar(30) DEFAULT NULL,
		  inserted_timestamp int(11) DEFAULT NULL,
		  num_checkins int(11) DEFAULT '0',
		  last_alert_timestamp int(11) DEFAULT NULL,
		  last_checkin_timestamp int(11) DEFAULT NULL,
		  PRIMARY KEY (id),
		  UNIQUE KEY uniq_app (app,component)
		) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;
			`
	_, err = db.Exec(sql)
	if err != nil {
		panic(err)
		return
	}

	housekeeping := `CREATE TABLE IF NOT EXISTS housekeeping (
		  id int(11) NOT NULL AUTO_INCREMENT,
		  app varchar(30) DEFAULT NULL,
		  component varchar(30) DEFAULT NULL,
		  notes text,
		  last_checkin_timestamp int(11) DEFAULT NULL,
		  insert_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		  PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8;`
	_, err = db.Exec(housekeeping)
	if err != nil {
		panic(err)
		return
	}

	nodes := `CREATE TABLE IF NOT EXISTS nodes (
			  id int(11) unsigned NOT NULL AUTO_INCREMENT,
			  ip_address varchar(20) DEFAULT NULL,
			  node_id int(30) DEFAULT NULL,
			  PRIMARY KEY (id),
			  UNIQUE KEY uniq_ip (ip_address)
			) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8;`
	_, err = db.Exec(nodes)
	if err != nil {
		panic(err)
		return
	}

	// store gotel as the initial application to monitor
	l.info("Starting to bootstrap worker/coordinator reservations...")
	tomorrow := time.Now().Add(24 * time.Hour).UTC().Unix()
	now := time.Now().UTC().Unix()
	gotelApp := `INSERT INTO reservations (app, component, owner, notify, frequency, time_units, inserted_timestamp, last_checkin_timestamp)
		VALUES ('gotel', 'coordinator', ?, ?, 5, 'minutes', ?, ?) ON DUPLICATE KEY UPDATE owner=?`
	_, err = db.Exec(gotelApp, conf.Main.GotelOwnerEmail, conf.Main.GotelOwnerEmail, now, tomorrow, conf.Main.GotelOwnerEmail)
	if err != nil {
		l.warn("storing gotel/coordinator as initial app [%q]", err)
	} else {
		l.info("Inserted gotel/coordinator as first app to monitor")
	}

	gotelAppWorker := `INSERT INTO reservations (app, component, owner, notify, frequency, time_units, inserted_timestamp, last_checkin_timestamp)
		VALUES ('gotel', 'worker', ?, ?, 5, 'minutes', ?, ?) ON DUPLICATE KEY UPDATE owner=?`
	_, err = db.Exec(gotelAppWorker, conf.Main.GotelOwnerEmail, conf.Main.GotelOwnerEmail, now, tomorrow, conf.Main.GotelOwnerEmail)
	if err != nil {
		l.warn("storing gotel/worker as initial app [%v]", err)
	} else {
		l.info("Inserted gotel/worker as first app to monitor")
	}
}
