package main

import (
	"flag"
	"github.com/CrowdStrike/gotel"
	"github.com/ParsePlatform/go.flagenv"

	_ "github.com/go-sql-driver/mysql"
	"log"

	"time"
)

func main() {

	dbHost := flag.String("GOTEL_DB_HOST", "127.0.0.1", "Host of the DB instance")
	dbUser := flag.String("GOTEL_DB_USER", "root", "DB User")
	dbPass := flag.String("GOTEL_DB_PASSWORD", "", "DB Pass")
	confPath := flag.String("GOTEL_CONFIG_PATH", "./gotel.gcfg", "config file path")
	sysLogEnabled := flag.Bool("GOTEL_SYSLOG", false, "Use syslog for output logging")
	htmlPath := flag.String("GOTEL_HTML_PATH", "../../", "Path to the public folder for storing HTML files")
	flag.Parse()
	flagenv.Parse()

	config := gotel.NewConfig(*confPath, *sysLogEnabled)
	db := gotel.InitDb(*dbHost, *dbUser, *dbPass, config)
	defer db.Close()

	ge := &gotel.Endpoint{Db: db}

	gotel.InitializeMonitoring(config)

	// set up a ticker that every n seconds we check the jobs that should have checked in
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for t := range ticker.C {
			log.Println("Running job checker at ", t)
			gotel.Monitor(ge.Db)
		}
	}()
	gotel.InitAPI(ge, 8080, *htmlPath)
}
