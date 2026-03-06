module wisemed-labreaders/config

go 1.19

replace wisemed-labreaders/sqlitewrapper => ../sqlitewrapper

replace wisemed-labreaders/general => ../general

require (
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000 // indirect
)
