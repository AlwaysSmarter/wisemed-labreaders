package main

import (
	log "github.com/sirupsen/logrus"
	"os"
	"time"
	"wisemed-labreaders/comm/tcpip"
	"wisemed-labreaders/implementation"
	"wisemed-labreaders/server/srv"
	"wisemed-labreaders/sqlitewrapper"
)

var mySrv *srv.SIUIAPIServerType

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.WarnLevel)
}
func main() {
	err := implementation.CheckParameters(APP_VERSION, APP_ANALYZER_NAME)
	if err != nil {
		os.Exit(-1)
	}
	tcpip.TCPIPReadDealineItmeout = 500 * time.Millisecond
	tcpip.TCPIPWriteDealineItmeout = 18000 * time.Millisecond

	sessMngr, err := srv.NewSessionManager("memory", "wmrsessionid", 3600)
	if err != nil {
		panic(err)
	}

	mySrv = &srv.SIUIAPIServerType{
		CommHandlerCreator: createCommHandler,
		SessManager:        sessMngr,
	}
	mySrv.Run(APP_SERIAL_NO, APP_ANALYZER_NAME, APP_ANALYZER_TYPE, APP_IMPL_PATH, APP_ANALYZER_API_KEY)

	defer sqlitewrapper.SQLITEDatabase.CloseDatabase()
}
