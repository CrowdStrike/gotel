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
	JobID                int    `json:"job_id"`
	Owner                string `json:"owner"`
	Notify               string `json:"notify"`
	AlertMessage         string `json:"alert_msg"`
	App                  string `json:"app"`
	Component            string `json:"component"`
	Frequency            int    `json:"frequency"`
	TimeUnits            string `json:"time_units"`
	LastCheckin          int64  `json:"last_checkin"`
	LastCheckinStr       string `json:"last_checkin_str"` // human readable time
	TimeSinceLastCheckin string `json:"time_since_last_checkin"`
	FailingSLA           bool   `json:"failing_sla"`
	NumCheckins          int    `json:"number_of_checkins"`
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

// InitDb initializes and then bootstraps the database
func InitDb(host, user, pass string, conf Config) *sql.DB {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:3306)/gotel", user, pass, host))
	if err != nil {
		panic(err)
	}
	err = db.Ping()
	if err != nil {
		panic(fmt.Sprintf("Unable to ping the DB at host [%s] user [%s]: %v", host, user, err))
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
		return false, errors.New("unable to store reservations for less than 10 seconds at this time, for no real reason")
	}

	stmt, err := db.Prepare(`INSERT INTO reservations(app, component, owner, notify, alert_msg, frequency, time_units, inserted_timestamp, last_checkin_timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE notify=?, alert_msg=?, frequency=?, time_units=?
		`)

	if err != nil {
		l.warn("unable to prepare statement %s", err)
		return false, errors.New("Unable to save record")
	}
	defer stmt.Close()

	res, err := stmt.Exec(r.App, r.Component, r.Owner, r.Notify, r.AlertMessage, r.Frequency, r.TimeUnits, now, tomorrow,
		r.Notify, r.AlertMessage, r.Frequency, r.TimeUnits)
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
	stmt, err := db.Prepare("INSERT INTO housekeeping(app, component, notes, last_checkin_timestamp) VALUES (?, ?, ?, ?)")
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

func bootstrapDb(db *sql.DB, conf Config) {

	l.info("Bootstrapping GoTel DB tables")
	versions := getTablesVersions(db)
	tx, err := db.Begin()
	if err != nil {
		l.err("Could not start transaction [%v]", err)
		panic(err)
	}

	if ver, hasTable := versions["tables_versions"]; !hasTable {
		doTxQuery(tx, `CREATE TABLE IF NOT EXISTS tables_versions (
		  id int(11) unsigned NOT NULL AUTO_INCREMENT,
		  table_name varchar(30) NOT NULL,
		  table_version int(11) NOT NULL DEFAULT 0,
		  PRIMARY KEY (id),
		  UNIQUE KEY uniq_name (table_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
		setTableVersion(tx, "tables_versions", 0)
	} else {
		l.info("tables_versions is version %d", ver)
	}

	if ver, hasTable := versions["alerts"]; !hasTable {
		doTxQuery(tx, `CREATE TABLE IF NOT EXISTS alerts (
		  id int(11) unsigned NOT NULL AUTO_INCREMENT,
		  app varchar(30) DEFAULT NULL,
		  component varchar(30) DEFAULT NULL,
		  alert_time int(11) DEFAULT NULL,
		  alerters text DEFAULT NULL,
		  insert_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		  PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
		setTableVersion(tx, "alerts", 0)
	} else {
		l.info("alerts is version %d", ver)
	}

	if ver, hasTable := versions["reservations"]; !hasTable {
		doTxQuery(tx, `CREATE TABLE IF NOT EXISTS reservations (
		  id int(11) NOT NULL AUTO_INCREMENT,
		  app varchar(150) DEFAULT NULL,
		  component varchar(150) DEFAULT NULL,
		  owner text DEFAULT NULL,
		  notify text DEFAULT NULL,
		  alert_msg text DEFAULT NULL,
		  frequency int(11) DEFAULT NULL,
		  time_units varchar(30) DEFAULT NULL,
		  inserted_timestamp int(11) DEFAULT NULL,
		  num_checkins int(11) DEFAULT '0',
		  last_alert_timestamp int(11) DEFAULT NULL,
		  last_checkin_timestamp int(11) DEFAULT NULL,
		  PRIMARY KEY (id),
		  UNIQUE KEY uniq_app (app,component)
		) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;`)
		setTableVersion(tx, "reservations", 1)
	} else {
		l.info("reservations is version %d", ver)

		if ver < 1 {
			doTxQuery(tx, `ALTER TABLE reservations ADD COLUMN alert_msg text DEFAULT NULL AFTER notify;`)
			setTableVersion(tx, "reservations", 1)
		}
	}

	if ver, hasTable := versions["housekeeping"]; !hasTable {
		doTxQuery(tx, `CREATE TABLE IF NOT EXISTS housekeeping (
		  id int(11) NOT NULL AUTO_INCREMENT,
		  app varchar(30) DEFAULT NULL,
		  component varchar(30) DEFAULT NULL,
		  notes text,
		  last_checkin_timestamp int(11) DEFAULT NULL,
		  insert_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		  PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
		setTableVersion(tx, "housekeeping", 0)
	} else {
		l.info("housekeeping is version %d", ver)
	}

	if ver, hasTable := versions["nodes"]; !hasTable {
		doTxQuery(tx, `CREATE TABLE IF NOT EXISTS nodes (
		  id int(11) unsigned NOT NULL AUTO_INCREMENT,
		  ip_address varchar(20) DEFAULT NULL,
		  node_id int(30) DEFAULT NULL,
		  PRIMARY KEY (id),
		  UNIQUE KEY uniq_ip (ip_address)
		) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8;`)
		setTableVersion(tx, "nodes", 0)
	} else {
		l.info("nodes is version %d", ver)
	}

	// store gotel as the initial application to monitor
	l.info("Starting to bootstrap worker/coordinator reservations...")
	tomorrow := time.Now().Add(24 * time.Hour).UTC().Unix()
	now := time.Now().UTC().Unix()
	gotelApp := `INSERT INTO reservations (app, component, owner, notify, frequency, time_units, inserted_timestamp, last_checkin_timestamp)
		VALUES ('gotel', 'coordinator', ?, ?, 5, 'minutes', ?, ?) ON DUPLICATE KEY UPDATE owner=?`
	_, err = tx.Exec(gotelApp, conf.Main.GotelOwnerEmail, conf.Main.GotelOwnerEmail, now, tomorrow, conf.Main.GotelOwnerEmail)
	if err != nil {
		l.warn("storing gotel/coordinator as initial app [%q]", err)
	} else {
		l.info("Inserted gotel/coordinator as first app to monitor")
	}

	gotelAppWorker := `INSERT INTO reservations (app, component, owner, notify, frequency, time_units, inserted_timestamp, last_checkin_timestamp)
		VALUES ('gotel', 'worker', ?, ?, 5, 'minutes', ?, ?) ON DUPLICATE KEY UPDATE owner=?`
	_, err = tx.Exec(gotelAppWorker, conf.Main.GotelOwnerEmail, conf.Main.GotelOwnerEmail, now, tomorrow, conf.Main.GotelOwnerEmail)
	if err != nil {
		l.warn("storing gotel/worker as initial app [%v]", err)
	} else {
		l.info("Inserted gotel/worker as first app to monitor")
	}

	err = tx.Commit()
	if err != nil {
		l.err("Could not commit transaction [%v]", err)
		panic(err)
	}
	l.info("DB ready")
}

func getTablesVersions(db *sql.DB) map[string]int {
	var (
		tbl string
		ver int
	)
	versions := make(map[string]int)

	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema='gotel'`)
	if err != nil {
		l.err("Unable to select tables [%v]", err)
		panic(err)
	}

	for rows.Next() {
		err = rows.Scan(&tbl)
		if err != nil {
			l.err("Unable to scan table rows [%v]", err)
			panic(err)
		}
		versions[tbl] = 0
	}
	err = rows.Close()
	if err != nil {
		l.err("Unable to close query [%v]", err)
	}

	if _, hasKey := versions["tables_versions"]; hasKey {
		rows, err = db.Query(`SELECT table_name, table_version FROM tables_versions`)
		if err != nil {
			l.err("Unable to select tables versions [%v]", err)
			panic(err)
		}

		for rows.Next() {
			err = rows.Scan(&tbl, &ver)
			if err != nil {
				l.err("Unable to scan table version [%v]", err)
				panic(err)
			}
			versions[tbl] = ver
		}
		err = rows.Close()
		if err != nil {
			l.err("Unable to close query [%v]", err)
		}
	}

	return versions
}

func setTableVersion(tx *sql.Tx, tbl string, ver int) {
	tableVersion := `INSERT INTO tables_versions (table_name, table_version)
		VALUES (?, ?) ON DUPLICATE KEY UPDATE table_version=?`
	_, err := tx.Exec(tableVersion, tbl, ver, ver)
	if err != nil {
		l.err("Could not set table version [%v]", err)
		err = tx.Rollback()
		if err != nil {
			l.err("Could not roll back transaction [%v]", err)
		}
		panic(err)
	} else {
		l.info("Updated %s to version %d", tbl, ver)
	}
}

func doTxQuery(tx *sql.Tx, query string) sql.Result {
	res, err := tx.Exec(query)

	if err != nil {
		l.err("Could not execute query [%v]", err)
		err = tx.Rollback()
		if err != nil {
			l.err("Could not roll back transaction [%v]", err)
		}
		panic(err)
	}

	return res
}
