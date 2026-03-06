module wisemed-labreaders/comm

go 1.19

replace wisemed-labreaders/config => ../config

replace wisemed-labreaders/sqlitewrapper => ../sqlitewrapper

replace wisemed-labreaders/general => ../general

replace wisemed-labreaders/comm/serial => ./serial

replace wisemed-labreaders/comm/tcpip => ./tcpip

require (
	wisemed-labreaders/comm/serial v0.0.0-00010101000000-000000000000
	wisemed-labreaders/comm/tcpip v0.0.0-00010101000000-000000000000
	wisemed-labreaders/config v0.0.0-00010101000000-000000000000
)

require (
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000 // indirect
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000 // indirect
)
