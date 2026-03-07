module wisemed-labreaders/config

go 1.19

replace wisemed-labreaders/sqlitewrapper => ../sqlitewrapper

replace wisemed-labreaders/general => ../general

require (
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f
	gopkg.in/yaml.v3 v3.0.1
	wisemed-labreaders/sqlitewrapper v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000 // indirect
)
