module wisemed-labreaders/implementation/sysmex-ca600

go 1.19

replace wisemed-labreaders/implementation => ../

replace wisemed-labreaders/comm => ../../comm

replace wisemed-labreaders/comm/serial => ../../comm/serial

replace wisemed-labreaders/comm/tcpip => ../../comm/tcpip

replace wisemed-labreaders/config => ../../config

replace wisemed-labreaders/general => ../../general

replace wisemed-labreaders/protocols => ../../protocols

replace wisemed-labreaders/protocols/astm => ../../protocols/astm

replace wisemed-labreaders/server => ../../server

replace wisemed-labreaders/server/srv => ../../server/srv

replace wisemed-labreaders/sqlitewrapper => ../../sqlitewrapper

replace wisemed-labreaders/wisemed => ../../wisemed

require (
	github.com/sirupsen/logrus v1.9.0
	wisemed-labreaders/config v0.0.0-00010101000000-000000000000
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000
	wisemed-labreaders/implementation v0.0.0-00010101000000-000000000000
	wisemed-labreaders/protocols v0.0.0-00010101000000-000000000000
	wisemed-labreaders/protocols/astm v0.0.0-00010101000000-000000000000
	wisemed-labreaders/server/srv v0.0.0-00010101000000-000000000000
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rs/cors v1.8.3 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07 // indirect
	go.bug.st/serial v1.6.1 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/sys v0.13.0 // indirect
	wisemed-labreaders/comm v0.0.0-00010101000000-000000000000 // indirect
	wisemed-labreaders/comm/serial v0.0.0-00010101000000-000000000000 // indirect
	wisemed-labreaders/comm/tcpip v0.0.0-00010101000000-000000000000 // indirect
	wisemed-labreaders/wisemed v0.0.0-00010101000000-000000000000 // indirect
)
