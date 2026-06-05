package main

import (
	"flag"
	"fmt"
	"log"

	"wisemed-labreaders/readersv3/apps/runner"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

func main() {
	cfgPath := flag.String("config", "deployments/config.yaml", "Path to reader config")
	modulesHeadless := flag.Bool("headless", false, "Run as a background service/daemon and return the shell prompt")
	headlessChild := flag.Bool("headless-child", false, "Internal flag used by the background launcher")
	installService := flag.Bool("installservice", false, "Install the application as an operating system service")
	reconfigure := flag.Bool("reconfigure", false, "Run interactive bootstrap/configuration wizard before starting")
	showLog := flag.Bool("showlog", false, "Print runtime logs to console in addition to the daily log file")
	showVersion := flag.Bool("version", false, "Print application version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(appmeta.CurrentVersion())
		return
	}
	modules := []string{"local-http", "storage-sqlite", "events", "wisemed-api", "wisemed-ws", "login", "help", "dashboard", "analytes", "analyte-management", "qc", "result-sync", "stats", "daily-orders", "transport-file", "protocol-analytikjena-plasmaquantms-elite"}
	if err := runner.Run(*cfgPath, modules, runner.RunOptions{Headless: *modulesHeadless, HeadlessChild: *headlessChild, InstallService: *installService, Reconfigure: *reconfigure, ShowLog: *showLog}); err != nil {
		log.Fatal(err)
	}
}
