module wisemed-labreaders/comm/serial

go 1.19

replace wisemed-labreaders/config => ../../config

replace wisemed-labreaders/sqlitewrapper => ../../sqlitewrapper

replace wisemed-labreaders/general => ../../general

require (
	go.bug.st/serial v1.6.1
	wisemed-labreaders/config v0.0.0-00010101000000-000000000000
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/sys v0.13.0 // indirect
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000 // indirect
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000 // indirect
)
