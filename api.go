package gotel

import (
	"errors"
	"fmt"
	"github.com/emicklei/go-restful"
	"log"
	"net/http"
	"time"
)

// Response will hold a response sent back to the caller
type Response map[string]interface{}

func (ge *Endpoint) makeReservation(req *restful.Request, resp *restful.Response) {
	res := new(reservation)
	err := req.ReadEntity(&res)
	if err != nil {
		l.err("Unable to accept reservation")
	}

	err = validatereservation(res)
	if err != nil {
		l.warn("Invalid reservations [%q]", res)
		resp.WriteHeader(http.StatusBadRequest)
		resp.WriteAsJson(fmt.Sprintf("Unable to store reservation, validation failure [%v]", err))
		return
	}

	l.info("%q", res)

	_, err = storeReservation(ge.Db, res)
	if err != nil {
		l.err("Unable to store reservation %v", res)
		resp.WriteHeader(http.StatusBadRequest)
		resp.WriteAsJson("Unable to store reservation")
		return
	}
	resp.WriteAsJson("OK")
}

func (ge *Endpoint) listReservations(req *restful.Request, resp *restful.Response) {
	query := "select id, app, component, owner, notify, frequency, time_units, last_checkin_timestamp from reservations"
	rows, err := ge.Db.Query(query)
	if err != nil {
		l.err("Unable to list reservations [%v]", err)
		r := Response{"success": false, "message": "Unable to list reservations"}
		resp.WriteAsJson(r)
		return
	}
	reservations := []reservation{}
	defer rows.Close()
	for rows.Next() {
		res := reservation{}
		rows.Scan(&res.JobID, &res.App, &res.Component, &res.Owner, &res.Notify, &res.Frequency, &res.TimeUnits, &res.LastCheckin)
		reservations = append(reservations, res)
	}
	result := Response{"success": true, "result": reservations}
	resp.WriteAsJson(result)
	return
}

func (ge *Endpoint) doCheckin(req *restful.Request, resp *restful.Response) {
	c := new(checkin)
	err := req.ReadEntity(&c)
	if err != nil {
		l.err("Unable to accept checkin for %v", c)
		r := Response{"success": false, "message": "Unable to checkin: " + c.App}
		resp.WriteAsJson(r)
		return
	}

	now := time.Now().UTC().Unix()

	_, err = storeCheckin(ge.Db, *c, now)
	if err != nil {
		l.err("Unable to save checkin for %v", c)
		r := Response{"success": false, "message": "Unable to save checkin: " + c.App}
		resp.WriteAsJson(r)
		return
	}

	_, err = logHouseKeeping(ge.Db, *c, now)
	if err != nil {
		l.err("Unable to save checkin for %v", c)
		r := Response{"success": false, "message": "Unable to save checkin: " + c.App}
		resp.WriteAsJson(r)
		return
	}
	l.info("app [%s] component [%s] checked in %v", c.App, c.Component, time.Now())
	r := Response{"success": true, "message": "Application checked in: " + c.App}
	resp.WriteAsJson(r)
}

// used when you know your service will be offline for a bit and you want to pause alerts
func (ge *Endpoint) doPause(req *restful.Request, resp *restful.Response) {
	p := new(pause)
	err := req.ReadEntity(&p)
	if err != nil {
		l.err("Unable to accept pause for %v", p)
		r := Response{"success": false, "message": "Unable to checkin: " + p.App}
		resp.WriteAsJson(r)
		return
	}

	_, err = storePause(ge.Db, p)
	if err != nil {
		l.err("Unable to save pause for %v", p)
		r := Response{"success": false, "message": "Unable to save checkin: " + p.App}
		resp.WriteAsJson(r)
		return
	}

	r := Response{"success": true, "message": "Application paused: " + p.App}
	resp.WriteAsJson(r)

}

// used when you know your service will be offline for a bit and you want to pause alerts
func (ge *Endpoint) doCheckOut(req *restful.Request, resp *restful.Response) {
	p := new(checkOut)
	err := req.ReadEntity(&p)
	if err != nil {
		l.err("Unable to accept checkout for %v error [%s]", p, err)
		r := Response{"success": false, "message": "Unable to checkout: " + p.App}
		resp.WriteAsJson(r)
		return
	}
	_, err = storeCheckOut(ge.Db, p)
	if err != nil {
		l.err("Unable to save checkout for %v", p)
		r := Response{"success": false, "message": "Unable to save checkout: " + p.App}
		resp.WriteAsJson(r)
		return
	}
	r := Response{"success": true, "message": fmt.Sprintf("Application Removed [%s/%s] ", p.App, p.Component)}
	resp.WriteAsJson(r)
}

// IsCoordinator returns true if this node is the coordinator
func (ge *Endpoint) IsCoordinator(req *restful.Request, resp *restful.Response) {
	resp.WriteEntity(coordinator)
}

func validatereservation(res *reservation) error {
	timeUnits := map[string]int{"seconds": 1, "minutes": 1, "hours": 1}
	_, ok := timeUnits[res.TimeUnits]
	if !ok {
		return errors.New("Invalid time_units passed in")
	}
	return nil
}

// InitAPI initializes the webservice on the specific port
func InitAPI(ge *Endpoint, port int) {
	ws := new(restful.WebService)
	ws.
		Path("/").
		// You can specify consumes and produces per route as well.
		Consumes(restful.MIME_JSON, restful.MIME_XML).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.POST("/reservation").To(ge.makeReservation).Produces(restful.MIME_JSON))
	ws.Route(ws.GET("/reservation").To(ge.listReservations).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/checkin").To(ge.doCheckin).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/checkout").To(ge.doCheckOut).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/pause").To(ge.doPause).Produces(restful.MIME_JSON))
	ws.Route(ws.GET("/is-coordinator").To(ge.IsCoordinator).Produces(restful.MIME_JSON))

	restful.Add(ws)
	server := http.ListenAndServe(":8080", nil)
	log.Panic(server)
}
