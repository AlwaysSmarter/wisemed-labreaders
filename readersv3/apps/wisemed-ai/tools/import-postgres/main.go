// Import only a sibling utility's PostgreSQL connection into a protected local config.
package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: import-postgres <existing SIUI config.yaml> <local AI config.yaml>")
		os.Exit(2)
	}
	if e := run(os.Args[1], os.Args[2]); e != nil {
		fmt.Fprintln(os.Stderr, "PostgreSQL import failed: check source/destination configuration")
		os.Exit(1)
	}
	fmt.Println("PostgreSQL connection imported; destination mode 0600. No credentials printed.")
}
func run(src, dest string) error {
	raw, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	var source map[string]any
	if e = yaml.Unmarshal(raw, &source); e != nil {
		return e
	}
	mods, _ := source["modules"].(map[string]any)
	s, _ := mods["siui-data"].(map[string]any)
	v := func(k string) string { return strings.TrimSpace(fmt.Sprint(s[k])) }
	for _, k := range []string{"postgres_host", "postgres_database", "postgres_user", "postgres_password"} {
		if v(k) == "" || v(k) == "<nil>" {
			return fmt.Errorf("missing connection setting")
		}
	}
	u := url.URL{Scheme: "postgres", Host: v("postgres_host") + ":" + v("postgres_port"), Path: "/" + v("postgres_database"), User: url.UserPassword(v("postgres_user"), v("postgres_password"))}
	q := u.Query()
	q.Set("sslmode", v("postgres_sslmode"))
	if v("postgres_root_cert") != "" && v("postgres_root_cert") != "<nil>" {
		q.Set("sslrootcert", v("postgres_root_cert"))
	}
	u.RawQuery = q.Encode()
	raw, e = os.ReadFile(dest)
	if e != nil {
		return e
	}
	var cfg map[string]any
	if e = yaml.Unmarshal(raw, &cfg); e != nil {
		return e
	}
	modules := cfg["modules"].(map[string]any)
	ai := modules["wisemed-ai"].(map[string]any)
	ai["database_url"] = u.String()
	raw, e = yaml.Marshal(cfg)
	if e != nil {
		return e
	}
	if e = os.WriteFile(dest, raw, 0600); e != nil {
		return e
	}
	return os.Chmod(dest, 0600)
}
