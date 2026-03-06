module wisemed-labreaders/protocols/hl7

go 1.19

replace wisemed-labreaders/config => ../../config

replace wisemed-labreaders/general => ../../general

replace wisemed-labreaders/sqlitewrapper => ../../sqlitewrapper
replace wisemed-labreaders/wisemed => ../../wisemed

replace wisemed-labreaders/hl7/hl7_handlers => ./hl7_handlers

replace wisemed-labreaders/hl7/hl7_segments => ./hl7_segments

require (
	github.com/lenaten/hl7 v0.0.0-20181009090854-63c5c49a56d9
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
)
