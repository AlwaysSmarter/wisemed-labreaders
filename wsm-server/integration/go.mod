module wisemed-labreaders/serverlast/wsm-server/integration

go 1.24.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
	wisemed-labreaders/readersv3 v0.0.0
	wisemed-labreaders/serverlast/wsm-server v0.0.0
)

require (
	github.com/go-chi/chi/v5 v5.3.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace wisemed-labreaders/serverlast/wsm-server => ..

replace wisemed-labreaders/readersv3 => ../../readersv3
