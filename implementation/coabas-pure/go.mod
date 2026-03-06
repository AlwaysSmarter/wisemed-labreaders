module wisemed-labreaders/implementation/cobas-pure

go 1.19

replace wisemed-labreaders/implementation => ../

replace wisemed-labreaders/comm => ../../comm

replace wisemed-labreaders/comm/serial => ../../comm/serial

replace wisemed-labreaders/comm/tcpip => ../../comm/tcpip

replace wisemed-labreaders/config => ../../config

replace wisemed-labreaders/general => ../../general

replace wisemed-labreaders/protocols => ../../protocols

replace wisemed-labreaders/protocols/hl7 => ../../protocols/hl7

replace wisemed-labreaders/protocols/hl7/hl7_handlers => ../../protocols/hl7/hl7_handlers

replace wisemed-labreaders/protocols/hl7/hl7_segments => ../../protocols/hl7/hl7_segments

replace wisemed-labreaders/server => ../../server

replace wisemed-labreaders/server/srv => ../../server/srv

replace wisemed-labreaders/sqlitewrapper => ../../sqlitewrapper

replace wisemed-labreaders/wisemed => ../../wisemed

require (
	github.com/sirupsen/logrus v1.9.0
	wisemed-labreaders/config v0.0.0-00010101000000-000000000000
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000
)

require (
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
)
