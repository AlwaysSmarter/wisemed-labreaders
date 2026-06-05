package builtin

import (
	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/modules/analytemanagement"
	"wisemed-labreaders/readersv3/modules/analytes"
	"wisemed-labreaders/readersv3/modules/appupdateserver"
	"wisemed-labreaders/readersv3/modules/barcodeprinter"
	"wisemed-labreaders/readersv3/modules/dailydetails"
	"wisemed-labreaders/readersv3/modules/dailyorders"
	"wisemed-labreaders/readersv3/modules/dashboard"
	"wisemed-labreaders/readersv3/modules/events"
	"wisemed-labreaders/readersv3/modules/help"
	"wisemed-labreaders/readersv3/modules/localhttp"
	"wisemed-labreaders/readersv3/modules/login"
	"wisemed-labreaders/readersv3/modules/protocols/analytikjenaplasmaquantmselite"
	"wisemed-labreaders/readersv3/modules/protocols/anatoliageneworks"
	astmproto "wisemed-labreaders/readersv3/modules/protocols/astm"
	"wisemed-labreaders/readersv3/modules/protocols/beoslcsv"
	"wisemed-labreaders/readersv3/modules/protocols/biosanhipompp96"
	"wisemed-labreaders/readersv3/modules/protocols/cary60uvvis"
	"wisemed-labreaders/readersv3/modules/protocols/gammavision"
	"wisemed-labreaders/readersv3/modules/protocols/genericfile"
	"wisemed-labreaders/readersv3/modules/protocols/irbiotyper"
	"wisemed-labreaders/readersv3/modules/protocols/labnovationld560"
	"wisemed-labreaders/readersv3/modules/protocols/seegeneexcel"
	"wisemed-labreaders/readersv3/modules/protocols/shimatzugeneric"
	"wisemed-labreaders/readersv3/modules/protocols/shimatzutocl"
	"wisemed-labreaders/readersv3/modules/protocols/tricarb5110tr"
	"wisemed-labreaders/readersv3/modules/qc"
	"wisemed-labreaders/readersv3/modules/resultsync"
	"wisemed-labreaders/readersv3/modules/stats"
	sqlitestorage "wisemed-labreaders/readersv3/modules/storage/sqlite"
	filetransport "wisemed-labreaders/readersv3/modules/transports/file"
	serialtransport "wisemed-labreaders/readersv3/modules/transports/serial"
	tcptransport "wisemed-labreaders/readersv3/modules/transports/tcpip"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
	"wisemed-labreaders/readersv3/modules/ws"
)

func RegisterAll(reg *module.Registry) {
	reg.Register("local-http", localhttp.New)
	reg.Register("storage-sqlite", sqlitestorage.New)
	reg.Register("events", events.New)
	reg.Register("wisemed-ws", ws.New)
	reg.Register("wisemed-api", wisemedapi.New)
	reg.Register("login", login.New)
	reg.Register("help", help.New)
	reg.Register("dashboard", dashboard.New)
	reg.Register("analytes", analytes.New)
	reg.Register("analyte-management", analytemanagement.New)
	reg.Register("qc", qc.New)
	reg.Register("result-sync", resultsync.New)
	reg.Register("stats", stats.New)
	reg.Register("daily-details", dailydetails.New)
	reg.Register("daily-orders", dailyorders.New)
	reg.Register("transport-file", filetransport.New)
	reg.Register("transport-serial", serialtransport.New)
	reg.Register("transport-tcpip", tcptransport.New)
	reg.Register("protocol-generic-file", genericfile.New)
	reg.Register("protocol-cary60-uvvis", cary60uvvis.New)
	reg.Register("protocol-analytikjena-plasmaquantms-elite", analytikjenaplasmaquantmselite.New)
	reg.Register("protocol-seegene-excel", seegeneexcel.New)
	reg.Register("protocol-beosl-csv", beoslcsv.New)
	reg.Register("protocol-biosan-hipo-mpp96", biosanhipompp96.New)
	reg.Register("protocol-gammavision", gammavision.New)
	reg.Register("protocol-shimatzu-tocl", shimatzutocl.New)
	reg.Register("protocol-shimatzu-generic", shimatzugeneric.New)
	reg.Register("protocol-tricarb-5110-tr", tricarb5110tr.New)
	reg.Register("protocol-ir-biotyper", irbiotyper.New)
	reg.Register("protocol-astm", astmproto.New)
	reg.Register("protocol-anatolia-geneworks", anatoliageneworks.New)
	reg.Register("protocol-labnovation-ld560", labnovationld560.New)
	reg.Register("barcode-printer", barcodeprinter.New)
	reg.Register("app-update-server", appupdateserver.New)
}
