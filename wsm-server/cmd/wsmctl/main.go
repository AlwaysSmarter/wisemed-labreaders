// wsmctl is an offline operator tool. It never contacts a running server.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/serverlast/wsm-server/internal/config"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: wsmctl keygen -out FILE | token -secret-file FILE -kid ID -tenant ID -issuer URL -subject ID -client ID -role ROLE -scopes LIST")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	if args[0] == "keygen" {
		out := fs.String("out", "", "New secret file (never overwritten)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *out == "" {
			return errors.New("-out required")
		}
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			return e
		}
		f, e := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.WriteString(base64.RawURLEncoding.EncodeToString(b) + "\n")
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	if args[0] != "token" {
		return errors.New("unknown command")
	}
	secret := fs.String("secret-file", "", "HMAC secret file; never put secrets in arguments")
	kid := fs.String("kid", "", "Key ID")
	tenant := fs.String("tenant", "", "Tenant ID")
	issuer := fs.String("issuer", "", "Issuer")
	aud := fs.String("audience", "wsm-server", "Audience")
	sub := fs.String("subject", "", "Subject")
	client := fs.String("client", "", "Client ID")
	role := fs.String("role", "", "browser, reader or service")
	equipment := fs.String("equipment", "", "WiseMED equipment ID (reader role)")
	reader := fs.String("reader", "", "Reader ID (reader role only)")
	label := fs.String("label", "", "Label")
	scopes := fs.String("scopes", "", "Comma-separated scopes")
	ttl := fs.Duration("ttl", 5*time.Minute, "Lifetime (must also satisfy server max)")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	if !config.ValidID(*kid) || !config.ValidID(*tenant) || !config.ValidID(*sub) || !config.ValidID(*client) || *issuer == "" || *aud == "" || *ttl < time.Second || *ttl > 24*time.Hour {
		return errors.New("invalid identity/issuer/audience/TTL")
	}
	if *role != "reader" && *role != "browser" && *role != "service" {
		return errors.New("invalid role")
	}
	if *role == "reader" && (!config.ValidID(*reader) || !config.ValidID(*equipment)) || *role != "reader" && *reader != "" {
		return errors.New("reader ID must match role")
	}
	list := strings.Split(*scopes, ",")
	for _, s := range list {
		if !config.ValidScope(s) {
			return errors.New("invalid scopes")
		}
	}
	b, e := os.ReadFile(*secret)
	if e != nil {
		return e
	}
	key := strings.TrimSpace(string(b))
	if len(key) < 32 {
		return errors.New("secret must be at least 32 bytes")
	}
	now := time.Now().UTC()
	c := server.AuthClaims{TenantID: *tenant, EquipmentID: *equipment, Role: *role, ClientID: *client, ReaderID: *reader, Label: *label, Scopes: list, RegisteredClaims: jwt.RegisteredClaims{Subject: *sub, Issuer: *issuer, Audience: jwt.ClaimStrings{*aud}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(*ttl))}}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	t.Header["kid"] = *kid
	s, e := t.SignedString([]byte(key))
	if e != nil {
		return e
	}
	fmt.Println(s)
	return nil
}
