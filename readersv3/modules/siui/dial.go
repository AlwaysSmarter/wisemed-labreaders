package siui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// Retry only the TCP connection establishment, before TLS or any HTTP bytes.
func dialCNAS(ctx context.Context, network, address string, dial func(context.Context, string, string) (net.Conn, error), logf func(string, ...interface{})) (net.Conn, error) {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		started := time.Now()
		if logf != nil {
			logf("SIUI TCP: conectare server=%s incercare=%d/3 limita=10s", address, attempt)
		}
		var conn net.Conn
		conn, err = dial(ctx, network, address)
		if err == nil {
			if logf != nil {
				logf("SIUI TCP: conectat server=%s durata=%.1fs", address, time.Since(started).Seconds())
			}
			return conn, nil
		}
		if conn != nil {
			conn.Close()
		}
		if logf != nil {
			logf("[ERROR] SIUI TCP: server=%s incercare=%d/3 durata=%.1fs eroare=%v", address, attempt, time.Since(started).Seconds(), err)
		}
		var op *net.OpError
		if !errors.As(err, &op) || op.Op != "dial" || !op.Timeout() || ctx.Err() != nil || attempt == 3 {
			break
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, err
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("CNAS TCP: conexiunea către %s nu a putut fi stabilită: %w", address, err)
}
