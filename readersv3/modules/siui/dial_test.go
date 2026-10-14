package siui

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestDialRetriesOnlyConnectTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"timeout", &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}, 3},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}, 1},
		{"read", &net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			_, err := dialCNAS(context.Background(), "tcp", "example.test:443", func(context.Context, string, string) (net.Conn, error) { count++; return nil, tc.err }, nil)
			if count != tc.want || !errors.Is(err, tc.err) {
				t.Fatal(count, err)
			}
		})
	}
}
func TestDialStopsAfterSuccessfulConnection(t *testing.T) {
	count := 0
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	conn, err := dialCNAS(context.Background(), "tcp", "example.test:8443", func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "example.test:8443" {
			t.Fatal("configured address changed", network, address)
		}
		count++
		if count == 1 {
			return nil, &net.OpError{Op: "dial", Err: context.DeadlineExceeded}
		}
		return a, nil
	}, nil)
	if err != nil || conn != a || count != 2 {
		t.Fatal(count, err)
	}
}
func TestDialHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dialCNAS(ctx, "tcp", "example.test:443", func(context.Context, string, string) (net.Conn, error) { t.Fatal("unexpected dial"); return nil, nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
