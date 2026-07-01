package bindguard

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
)

type Logger func(format string, args ...interface{})

func HandleAddressInUse(configPath, addr string, configUpdates map[string]interface{}, logf Logger) (string, bool, error) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	nextAddr, err := FindAlternativeAddress(addr)
	if err != nil {
		return "", true, err
	}
	nextHost, nextPort, err := net.SplitHostPort(nextAddr)
	if err != nil {
		return "", true, err
	}
	updates := map[string]interface{}{}
	for key, value := range configUpdates {
		updates[key] = value
	}
	for key := range updates {
		if strings.HasSuffix(key, ".address") {
			updates[key] = nextAddr
		}
		if strings.HasSuffix(key, ".host") {
			updates[key] = nextHost
		}
		if strings.HasSuffix(key, ".port") {
			updates[key] = nextPort
		}
	}
	if err := config.Update(configPath, updates); err != nil {
		return "", true, err
	}
	return nextAddr, true, nil
}

func IsAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(value, "address already in use") || strings.Contains(value, "only one usage of each socket address")
}

func FindAlternativeAddress(addr string) (string, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return "", err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	for candidate := port + 1; candidate <= port+50; candidate++ {
		nextAddr := net.JoinHostPort(host, strconv.Itoa(candidate))
		ln, err := net.Listen("tcp", nextAddr)
		if err != nil {
			continue
		}
		_ = ln.Close()
		return nextAddr, nil
	}
	return "", fmt.Errorf("nu am gasit port liber in intervalul %d-%d pentru %s", port+1, port+50, host)
}
