package main

import (
	"net"
	"testing"
)

func TestProbeAddress(t *testing.T) {
	for listen, want := range map[string]string{
		":8080":             "127.0.0.1:8080",
		"0.0.0.0:8080":      "127.0.0.1:8080",
		"[::]:8080":         "127.0.0.1:8080",
		"127.0.0.1:9000":    "127.0.0.1:9000",
		"192.168.1.20:8080": "192.168.1.20:8080",
		"[::1]:8080":        "[::1]:8080",
		"localhost:8080":    "localhost:8080",
	} {
		host, port, err := net.SplitHostPort(listen)
		if err != nil {
			t.Fatal(err)
		}
		if got := probeAddress(host, port); got != want {
			t.Errorf("probeAddress for %s = %s, want %s", listen, got, want)
		}
	}
}
