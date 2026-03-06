module wisemed-labreaders/sqlitewrapper

go 1.19

replace wisemed-labreaders/general => ../general

require (
	github.com/mattn/go-sqlite3 v1.14.16
	github.com/pkg/errors v0.9.1
	wisemed-labreaders/general v0.0.0-00010101000000-000000000000
)

require (
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	golang.org/x/crypto v0.5.0 // indirect
)
