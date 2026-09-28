// Command pump-it-up-tracker serves a read-only web UI for a personal log of
// Pump It Up scores.
//
// Usage:
//
//	pump-it-up-tracker [serve]           serve the UI (the default)
//	pump-it-up-tracker validate [FILE]   check a data file and report every problem
//	pump-it-up-tracker healthcheck       probe a running server (for container healthchecks)
//	pump-it-up-tracker version           print the version
//
// Configuration is read from the environment; see README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // lets TZ work in a distroless image

	"github.com/spatterIight/pump-it-up-tracker/internal/art"
	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
	"github.com/spatterIight/pump-it-up-tracker/internal/web"
)

var version = "dev"

type config struct {
	dataFile      string
	listenAddress string
	basePath      string
	artCacheDir   string
	artCustomDir  string
	artFetch      bool
	artSources    []string
	logLevel      slog.Level
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func loadConfig() (config, error) {
	c := config{
		dataFile:      env("PIU_TRACKER_DATA_FILE", "/config/tracker.json"),
		listenAddress: env("PIU_TRACKER_LISTEN_ADDRESS", ":8080"),
		basePath:      env("PIU_TRACKER_BASE_PATH", "/"),
		artCacheDir:   env("PIU_TRACKER_ART_CACHE_DIR", "/data/art"),
		artCustomDir:  env("PIU_TRACKER_ART_CUSTOM_DIR", "/art"),
	}
	fetch, err := strconv.ParseBool(env("PIU_TRACKER_ART_FETCH_ENABLED", "true"))
	if err != nil {
		return c, fmt.Errorf("PIU_TRACKER_ART_FETCH_ENABLED: %w", err)
	}
	c.artFetch = fetch
	for _, s := range strings.Split(env("PIU_TRACKER_ART_SOURCES", "piuscores,fandom"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.artSources = append(c.artSources, s)
		}
	}
	if err := c.logLevel.UnmarshalText([]byte(env("PIU_TRACKER_LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("PIU_TRACKER_LOG_LEVEL: %w", err)
	}
	return c, nil
}

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "validate":
		err = validate(os.Args[2:])
	case "healthcheck":
		err = healthcheck()
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Println("usage: pump-it-up-tracker [serve | validate [FILE] | healthcheck | version]")
	default:
		err = fmt.Errorf("unknown command %q (expected serve, validate, healthcheck or version)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pump-it-up-tracker:", err)
		os.Exit(1)
	}
}

func validate(args []string) error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	path := c.dataFile
	if len(args) > 0 {
		path = args[0]
	}
	t, err := tracker.LoadFile(path)
	if err != nil {
		return err
	}
	s := t.Stats(time.Now())
	fmt.Printf("%s is valid: %d plays of %d songs, %d of them failed\n", path, s.Plays, s.Songs, s.Fails)
	return nil
}

func healthcheck() error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(c.listenAddress)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://" + probeAddress(host, port) + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz answered %s", resp.Status)
	}
	return nil
}

// probeAddress is where the server listening on host:port can be reached
// from inside its container: loopback when it listens on every address,
// otherwise the address it listens on.
func probeAddress(host, port string) string {
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func serve() error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: c.logLevel}))
	slog.SetDefault(logger)

	t, err := tracker.LoadFile(c.dataFile)
	if err != nil {
		return fmt.Errorf("loading %s: %w", c.dataFile, err)
	}
	logger.Info("loaded score data", "file", c.dataFile, "plays", len(t.Plays), "songs", len(t.Songs), "version", version)

	userAgent := "pump-it-up-tracker/" + version + " (+https://github.com/spatterIight/pump-it-up-tracker)"
	client := &http.Client{Timeout: 20 * time.Second}
	var sources []art.Source
	for _, name := range c.artSources {
		switch name {
		case "piuscores":
			sources = append(sources, art.PIUScores{})
		case "fandom":
			sources = append(sources, art.Fandom{Client: client, UserAgent: userAgent})
		default:
			return fmt.Errorf("PIU_TRACKER_ART_SOURCES: unknown source %q (expected piuscores or fandom)", name)
		}
	}
	customDir := c.artCustomDir
	if fi, err := os.Stat(customDir); err != nil || !fi.IsDir() {
		customDir = ""
	}
	resolver := art.New(art.Options{
		CacheDir:     c.artCacheDir,
		CustomDir:    customDir,
		FetchEnabled: c.artFetch,
		Sources:      sources,
		Client:       client,
		UserAgent:    userAgent,
		RequestDelay: 250 * time.Millisecond,
		Logger:       logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	songs := make([]art.Song, 0, len(t.Songs))
	for _, s := range t.Songs {
		songs = append(songs, art.Song{Slug: s.Slug, Title: s.Title, Image: s.Image})
	}
	resolver.Start(ctx, songs)

	srv, err := web.New(web.Options{Tracker: t, Art: resolver, BasePath: c.basePath, Version: version, Logger: logger})
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              c.listenAddress,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", c.listenAddress, "base_path", web.NormalizeBasePath(c.basePath))
		errc <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
