package ecard

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type TDXIdentity struct {
	Contract   Contract `json:"contract"`
	Host       string   `json:"host"`
	UMEndpoint string   `json:"um_endpoint"`
}

func (i TDXIdentity) filename() (string, error) {
	if e := i.Contract.Validate(); e != nil {
		return "", e
	}
	if i.Host == "" || strings.ContainsAny(i.Host, "\r\n\x00") {
		return "", errors.New("source host required")
	}
	host, port, e := net.SplitHostPort(i.UMEndpoint)
	if e != nil || net.ParseIP(host) == nil || port == "" {
		return "", errors.New("UM endpoint must be resolved IP:port")
	}
	c := i.Contract
	date := strings.ReplaceAll(c.Date, "-", "")
	text := strings.Join([]string{c.Number, date, c.House, c.ProviderType, c.CUI, c.CIF, i.Host, i.UMEndpoint, "1"}, "\r\n")
	sum := sha1.Sum([]byte(text))
	return strings.ToUpper(hex.EncodeToString(sum[:])) + ".tdx", nil
}

type tdxMetadata struct {
	TerminalID string  `json:"terminal_id"`
	Test       bool    `json:"test_environment"`
	Profiles   []int32 `json:"profiles"`
}

func inspectTDX(raw []byte) (tdxMetadata, error) {
	out := tdxMetadata{}
	if len(raw) < 8 || len(raw) > 65536 || len(raw)%8 != 0 {
		return out, errors.New("invalid TDX size")
	}
	block, _ := des.NewCipher([]byte("lka@xx1-"))
	plain := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, make([]byte, 8)).CryptBlocks(plain, raw)
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	pad := int(plain[len(plain)-1])
	if pad < 1 || pad > 8 {
		return out, errors.New("unsupported TDX format")
	}
	for _, b := range plain[len(plain)-pad:] {
		if int(b) != pad {
			return out, errors.New("unsupported TDX padding")
		}
	}
	r := bytes.NewReader(plain[:len(plain)-pad])
	readString := func() (string, error) {
		var n uint32
		var shift uint
		for {
			b, e := r.ReadByte()
			if e != nil {
				return "", e
			}
			n |= uint32(b&127) << shift
			if b < 128 {
				break
			}
			shift += 7
			if shift > 28 {
				return "", errors.New("invalid length")
			}
		}
		if n > uint32(r.Len()) {
			return "", errors.New("invalid string")
		}
		b := make([]byte, n)
		_, e := r.Read(b)
		return string(b), e
	}
	var e error
	if out.TerminalID, e = readString(); e != nil {
		return out, e
	}
	if _, e = readString(); e != nil {
		return out, e
	}
	key, e := readString()
	if e != nil {
		return out, e
	}
	keyBytes, e := hex.DecodeString(key)
	if e != nil || len(keyBytes) != 16 || out.TerminalID == "" {
		return out, errors.New("incomplete enrollment")
	}
	for i := range keyBytes {
		keyBytes[i] = 0
	}
	key = ""
	flag, e := r.ReadByte()
	if e != nil || flag > 1 {
		return out, errors.New("invalid environment")
	}
	out.Test = flag == 1
	var count int32
	if binary.Read(r, binary.LittleEndian, &count) != nil || count < 1 || count > 1000 {
		return out, errors.New("invalid profiles")
	}
	for i := int32(0); i < count; i++ {
		var profile, n int32
		if binary.Read(r, binary.LittleEndian, &profile) != nil || binary.Read(r, binary.LittleEndian, &n) != nil || n < 1 || n > 1000 || int(n) > r.Len() {
			return out, errors.New("invalid profile key")
		}
		r.Seek(int64(n), 1)
		out.Profiles = append(out.Profiles, profile)
	}
	if r.Len() != 0 {
		return out, errors.New("unsupported trailing TDX data")
	}
	return out, nil
}
func (m *Module) handleTDX(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != "POST" || (action != "preview" && action != "migrate") {
		fail(w, 405, errors.New("use POST tdx/preview or tdx/migrate"))
		return
	}
	var in struct {
		Source        TDXIdentity `json:"source"`
		Filename      string      `json:"filename"`
		Data          string      `json:"data_base64"`
		ConfirmDigest string      `json:"confirm_digest"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	expected, e := in.Source.filename()
	if e != nil {
		fail(w, 400, e)
		return
	}
	if !strings.EqualFold(in.Filename, expected) {
		fail(w, 400, fmt.Errorf("source filename does not match source identity: expected %s; use the original contract number/date/type, exact source computer name and original resolved UM IP:port (not the current contract)", expected))
		return
	}
	raw, e := base64.StdEncoding.DecodeString(in.Data)
	if e != nil {
		fail(w, 400, e)
		return
	}
	meta, e := inspectTDX(raw)
	if e != nil {
		fail(w, 400, e)
		return
	}
	host, e := os.Hostname()
	if e != nil {
		fail(w, 500, e)
		return
	}
	settings := m.settingsSnapshot()
	addresses, e := net.DefaultResolver.LookupIPAddr(r.Context(), settings.UMHost)
	if e != nil || len(addresses) == 0 {
		fail(w, 400, errors.New("cannot resolve configured UM"))
		return
	}
	// The SDK uses the first DNS result. Never guess if DNS is ambiguous.
	if len(addresses) != 1 {
		fail(w, 409, errors.New("UM resolves to multiple addresses; configure the intended IP explicitly before migration"))
		return
	}
	target := TDXIdentity{Contract: in.Source.Contract, Host: host, UMEndpoint: net.JoinHostPort(addresses[0].IP.String(), fmt.Sprint(settings.UMPort))}
	name, e := target.filename()
	if e != nil {
		fail(w, 400, e)
		return
	}
	digest := sha256.New()
	digest.Write(raw)
	digest.Write([]byte(expected))
	digest.Write([]byte(name))
	confirmation := hex.EncodeToString(digest.Sum(nil))
	if action == "preview" {
		respond(w, 200, map[string]interface{}{"metadata": meta, "source": in.Source, "target": target, "target_filename": name, "confirm_digest": confirmation, "new_cnas_enrollment": false})
		return
	}
	if in.ConfirmDigest != confirmation {
		fail(w, 409, errors.New("preview confirmation mismatch"))
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 409, errors.New("TDX installation is available on the Windows target only"))
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.settings.UMHost != settings.UMHost || m.settings.UMPort != settings.UMPort {
		fail(w, 409, errors.New("UM settings changed; repeat migration preview"))
		return
	}
	if len(m.active) > 0 {
		fail(w, 409, errors.New("wait for card jobs before migrating"))
		return
	}
	root := os.Getenv("ProgramData")
	if root == "" {
		fail(w, 500, errors.New("ProgramData missing"))
		return
	}
	dir := filepath.Join(root, "Novensys.eCard.SDK", "TerminalData")
	if e = os.MkdirAll(dir, 0700); e != nil {
		fail(w, 500, e)
		return
	}
	path := filepath.Join(dir, name)
	if old, e := os.ReadFile(path); e == nil {
		if bytes.Equal(old, raw) {
			respond(w, 200, map[string]interface{}{"ok": true, "unchanged": true, "filename": name})
			return
		}
		backup := path + ".backup-" + time.Now().UTC().Format("20060102T150405.000000000")
		if e = os.WriteFile(backup, old, 0600); e != nil {
			fail(w, 500, errors.New("backup failed"))
			return
		}
	} else if !os.IsNotExist(e) {
		fail(w, 500, e)
		return
	}
	// Preserve the exact encrypted payload, including keys and terminal identity.
	tmp, e := os.CreateTemp(dir, ".tdx-*")
	if e != nil {
		fail(w, 500, e)
		return
	}
	temp := tmp.Name()
	defer os.Remove(temp)
	_, e = tmp.Write(raw)
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(temp, path)
	}
	if e != nil {
		fail(w, 500, errors.New("TDX installation failed; backup retained"))
		return
	}
	respond(w, 200, map[string]interface{}{"ok": true, "filename": name, "new_cnas_enrollment": false})
}
