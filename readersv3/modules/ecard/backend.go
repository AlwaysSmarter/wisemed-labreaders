package ecard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

type backend interface {
	Call(context.Context, map[string]interface{}, func(string) (string, error)) (Reply, error)
	Close()
}
type processBackend struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Scanner
	close  sync.Once
	cancel context.CancelFunc
}

func newBackend(parent context.Context, id string, s Settings) (backend, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("SDK requires Windows and .NET Framework 4.8")
	}
	dll := "Ceas.eCard.SDK.dll"
	if id == "thinkit-1.3.1.4" {
		dll = "titCeas.eCard.SDK.dll"
	}
	path := filepath.Join(s.SDKRoot, id, dll)
	if _, e := os.Stat(path); e != nil {
		return nil, errors.New("SDK payload missing")
	}
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, filepath.Join(s.HostDir, "ECardHost.exe"), path)
	cmd.Dir = s.HostDir
	cmd.Env = append(os.Environ(), "CEAS_UM_LOCAL=0")
	in, e := cmd.StdinPipe()
	if e != nil {
		cancel()
		return nil, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		cancel()
		return nil, e
	}
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		cancel()
		return nil, e
	}
	p := &processBackend{cmd: cmd, in: in, out: bufio.NewScanner(out), cancel: cancel}
	p.out.Buffer(make([]byte, 65536), 2<<20)
	return p, nil
}
func (p *processBackend) Close() {
	p.close.Do(func() { p.cancel(); _ = p.in.Close(); _ = p.cmd.Wait() })
}
func (p *processBackend) Call(ctx context.Context, request map[string]interface{}, pin func(string) (string, error)) (Reply, error) {
	stop := context.AfterFunc(ctx, p.cancel)
	defer stop()
	if e := json.NewEncoder(p.in).Encode(request); e != nil {
		return Reply{}, errors.New("host_write_failed")
	}
	for p.out.Scan() {
		var r Reply
		if e := json.Unmarshal(p.out.Bytes(), &r); e != nil {
			return Reply{}, errors.New("invalid_host_response")
		}
		if r.Event == "pin" {
			v, e := pin(r.Prompt)
			if e != nil {
				return Reply{}, e
			}
			if e = json.NewEncoder(p.in).Encode(map[string]string{"value": v}); e != nil {
				return Reply{}, errors.New("pin_delivery_failed")
			}
			v = ""
			continue
		}
		if r.Event != "" {
			continue
		}
		return r, nil
	}
	return Reply{}, errors.New("host_exited_or_timed_out")
}
