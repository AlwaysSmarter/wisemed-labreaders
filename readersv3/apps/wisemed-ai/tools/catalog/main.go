package main

import (
	"encoding/json"
	"os"
	"wisemed-labreaders/readersv3/modules/wisemedai"
)

func main() {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if e.Encode(wisemedai.Catalog()) != nil {
		os.Exit(1)
	}
}
