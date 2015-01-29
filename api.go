package gotel

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"
)

// Response will hold a response sent back to the caller
type Response map[string]interface{}

var validTimeUnits = map[string]int{"seconds": 1, "minutes": 1, "hours": 1}

func writeError(w http.ResponseWriter, e interface{}) {
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/json")
	if bytes, err := json.Marshal(e); err != nil {
		w.Write([]byte("Could not encode error"))
		return
	} else {
		w.Write(bytes)
		return
	}
}

func writeResponse(w http.ResponseWriter, e interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if bytes, err := json.Marshal(e); err != nil {
		w.Write([]byte("Could not encode response"))
	} else {
		w.Write(bytes)
		return
	}
}

func (ge *Endpoint) makeReservation(w http.ResponseWriter, req *http.Request) {
	res := new(reservation)
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&res)
	if err != nil {
		l.err("Unable to accept reservation")
	}

	err = validateReservation(res)
	if err != nil {
		l.warn("Invalid reservations [%q]", res)
		writeError(w, fmt.Sprintf("Unable to store reservation, validation failure [%v]", err))
		return
	}

	l.info("%q", res)

	_, err = storeReservation(ge.Db, res)
	if err != nil {
		l.err("Unable to store reservation %v", res)
		writeError(w, "Unable to store reservation")
		return
	}
	writeResponse(w, "OK")
}

func (ge *Endpoint) getReservations() ([]reservation, error) {
	query := "SELECT id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp, num_checkins FROM reservations ORDER BY last_checkin_timestamp DESC"
	rows, err := ge.Db.Query(query)
	if err != nil {
		return nil, err
	}
	reservations := []reservation{}
	defer rows.Close()
	for rows.Next() {
		res := reservation{}
		rows.Scan(&res.JobID, &res.App, &res.Component, &res.Owner, &res.Notify, &res.Frequency, &res.TimeUnits, &res.LastCheckin, &res.NumCheckins)
		lastCheckin := time.Unix(res.LastCheckin, 0)
		res.TimeSinceLastCheckin = RelTime(lastCheckin, time.Now(), "ago", "")
		res.LastCheckinStr = lastCheckin.Format(time.RFC1123)
		if FailsSLA(res) {
			res.FailingSLA = true
		} else {
			res.FailingSLA = false
		}
		reservations = append(reservations, res)
	}
	return reservations, nil
}

func (ge *Endpoint) listReservations(w http.ResponseWriter, req *http.Request) {
	reservations, err := ge.getReservations()
	if err != nil {
		l.err("Unable to list reservations [%v]", err)
		r := Response{"success": false, "message": "Unable to list reservations"}
		writeResponse(w, r)
		return
	}
	result := Response{"success": true, "result": reservations}
	writeResponse(w, result)
	return
}

func (ge *Endpoint) doCheckin(w http.ResponseWriter, req *http.Request) {
	c := new(checkin)
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&c)
	if err != nil {
		l.err("Unable to accept checkin for %v", c)
		r := Response{"success": false, "message": "Unable to checkin: " + c.App}
		writeResponse(w, r)
		return
	}

	now := time.Now().UTC().Unix()

	_, err = storeCheckin(ge.Db, *c, now)
	if err != nil {
		l.err("Unable to save checkin for %v", c)
		r := Response{"success": false, "message": "Unable to save checkin: " + c.App}
		writeResponse(w, r)
		return
	}

	_, err = logHouseKeeping(ge.Db, *c, now)
	if err != nil {
		l.err("Unable to save checkin for %v", c)
		r := Response{"success": false, "message": "Unable to save checkin: " + c.App}
		writeResponse(w, r)
		return
	}
	l.info("app [%s] component [%s] checked in %v", c.App, c.Component, time.Now())
	r := Response{"success": true, "message": "Application checked in: " + c.App}
	writeResponse(w, r)
}

// used when you know your service will be offline for a bit and you want to pause alerts
func (ge *Endpoint) doSnooze(w http.ResponseWriter, req *http.Request) {
	p := new(snooze)
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&p)
	if err != nil {
		l.err("Unable to accept snooze for %v", p)
		r := Response{"success": false, "message": "Unable to snooze: " + p.App}
		writeResponse(w, r)
		return
	}

	err = validateSnooze(p)
	if err != nil {
		l.warn("Invalid reservations [%q]", p)
		writeError(w, fmt.Sprintf("Unable to store snooze, validation failure [%v]", err))
		return
	}

	_, err = storeSnooze(ge.Db, p)
	if err != nil {
		l.err("Unable to save snooze for %v", p)
		r := Response{"success": false, "message": "Unable to save snooze: " + p.App}
		writeResponse(w, r)
		return
	}

	r := Response{"success": true, "message": "Application alerting paused: " + p.App}
	writeResponse(w, r)

}

func validateSnooze(snooze *snooze) error {
	_, ok := validTimeUnits[snooze.TimeUnits]
	if !ok {
		return errors.New("Invalid time_units passed in")
	}
	if snooze.Duration == 0 {
		return errors.New("")
	}
	return nil
}

// used when you know your service will be offline for a bit and you want to pause alerts
func (ge *Endpoint) doCheckOut(w http.ResponseWriter, req *http.Request) {
	p := new(checkOut)
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&p)
	if err != nil {
		l.err("Unable to accept checkout for %v error [%s]", p, err)
		r := Response{"success": false, "message": "Unable to checkout: " + p.App}
		writeResponse(w, r)
		return
	}
	_, err = storeCheckOut(ge.Db, p)
	if err != nil {
		l.err("Unable to save checkout for %v", p)
		r := Response{"success": false, "message": "Unable to save checkout: " + p.App}
		writeResponse(w, r)
		return
	}
	r := Response{"success": true, "message": fmt.Sprintf("Application Removed [%s/%s] ", p.App, p.Component)}
	writeResponse(w, r)
}

func (ge *Endpoint) isCoordinator(w http.ResponseWriter, req *http.Request) {
	writeResponse(w, coordinator)
}

func validateReservation(res *reservation) error {
	timeUnits := map[string]int{"seconds": 1, "minutes": 1, "hours": 1}
	_, ok := timeUnits[res.TimeUnits]
	if !ok {
		return errors.New("Invalid time_units passed in")
	}
	return nil
}

// InitAPI initializes the webservice on the specific port
func InitAPI(ge *Endpoint, port int, htmlPath string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			r := Response{"success": true, "message": "A-OK!"}
			writeResponse(w, r)
		}
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reservations, err := ge.getReservations()

			if err != nil {
				l.err(err.Error())
				r := Response{"success": false, "message": "Unable to server views"}
				writeResponse(w, r)
			} else {
				t, err := template.ParseFiles(htmlPath + "/public/view.html")
				if err != nil {
					l.err(err.Error())
				} else {
					t.Execute(w, &reservations)
				}

			}

		}
	})

	http.HandleFunc("/reservation", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ge.listReservations(w, r)
			return
		} else if r.Method == "POST" {
			ge.makeReservation(w, r)
			return
		}
		writeError(w, fmt.Sprintf("Invalid method %s", r.Method))
		return
	})
	http.HandleFunc("/checkin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			ge.doCheckin(w, r)
			return
		}
		writeError(w, fmt.Sprintf("Invalid method %s", r.Method))
		return
	})
	http.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			ge.doCheckOut(w, r)
			return
		}
		writeError(w, fmt.Sprintf("Invalid method %s", r.Method))
		return
	})
	http.HandleFunc("/snooze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			ge.doSnooze(w, r)
			return
		}
		writeError(w, fmt.Sprintf("Invalid method %s", r.Method))
		return
	})
	http.HandleFunc("/is-coordinator", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ge.isCoordinator(w, r)
			return
		}
		writeError(w, fmt.Sprintf("Invalid method %s", r.Method))
		return
	})

	server := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	log.Panic(server)
}
