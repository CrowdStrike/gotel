package main

import (
	"flag"
	"github.com/CrowdStrike/gotel"
	"github.com/ParsePlatform/go.flagenv"
	"github.com/emicklei/go-restful"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"time"
)

var (
	l *gotel.Logging
)

func init() {

}

func main() {

	dbHost := flag.String("GOTEL_DB_HOST", "127.0.0.1", "Host of the DB instance")
	dbUser := flag.String("GOTEL_DB_USER", "root", "DB User")
	dbPass := flag.String("GOTEL_DB_PASSWORD", "", "DB Pass")
	confPath := flag.String("GOTEL_CONFIG_PATH", "./gotel.gcfg", "config file path")
	sysLogEnabled := flag.Bool("GOTEL_SYSLOG", false, "Use syslog for output logging")
	flag.Parse()

	if err := flagenv.ParseEnv(); err != nil {
		panic(err)
	}

	l = &gotel.Logging{EnableSYSLOG: *sysLogEnabled}

	db, err := gotel.InitDb(*dbHost, *dbUser, *dbPass)
	if err != nil {
		panic("Unable to initialize Database properly, Exiting")
	}
	defer db.Close()

	ge := &gotel.GotelEndpoint{Db: db}

	config := gotel.Conf(*confPath, *sysLogEnabled)
	gotel.BootstrapDb(db, config)

	gotel.InitializeMonitoring(config)

	// set up a ticker that every n seconds we check the jobs that should have checked in
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for t := range ticker.C {
			log.Println("Running job checker at ", t)
			gotel.Monitor(ge.Db)
		}
	}()

	ws := new(restful.WebService)
	ws.
		Path("/").
		// You can specify consumes and produces per route as well.
		Consumes(restful.MIME_JSON, restful.MIME_XML).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.POST("/reservation").To(ge.Reservation).Produces(restful.MIME_JSON))
	ws.Route(ws.GET("/reservation").To(ge.ListReservations).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/checkin").To(ge.Checkin).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/checkout").To(ge.CheckOut).Produces(restful.MIME_JSON))
	ws.Route(ws.POST("/pause").To(ge.Pause).Produces(restful.MIME_JSON))
	ws.Route(ws.GET("/is-coordinator").To(ge.IsCoordinator).Produces(restful.MIME_JSON))

	restful.Add(ws)
	server := http.ListenAndServe(":8080", nil)
	log.Panic(server)
}
