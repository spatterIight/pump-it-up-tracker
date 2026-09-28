package main

import (
	"net"
	"strings"
	"testing"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
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

func TestSummary(t *testing.T) {
	for doc, want := range map[string]string{
		`{"schema_version": 1}`: "0 plays of 0 songs, 0 of them failed",
		`{"schema_version": 1, "scores": [
			{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204},
			{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true}
		]}`: "2 plays of 2 songs (all Phoenix), 1 of them failed",
		`{"schema_version": 1, "scores": [
			{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204},
			{"song": "Vook", "chart": "S7", "date": "2025-07-25", "version": "prime2", "score": 419400},
			{"song": "Vook", "chart": "S7", "date": "2025-07-26", "version": "prime2", "broken": true}
		]}`: "3 plays of 2 songs (1 Phoenix, 2 Prime 2), 1 of them failed",
	} {
		tr, err := tracker.Load(strings.NewReader(doc))
		if err != nil {
			t.Fatal(err)
		}
		if got := summary(tr); got != want {
			t.Errorf("summary = %q, want %q", got, want)
		}
	}
}
