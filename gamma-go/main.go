package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	moduleName  = "gamma-go"
	defaultPort = "8086"
	defaultEnv  = "dev"
	defaultDir  = "configuration"
)

func main() {
	env := envOrDefault("MARCHE_ENV", defaultEnv)
	cfg := loadConfig(env)
	port := cfg["server.port"]
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler(env, cfg))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Run server in a goroutine so the main goroutine can wait for a shutdown signal.
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("%s listening on :%s (env=%s)\n", moduleName, port, env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Block until an OS signal arrives or the server reports a fatal error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		fmt.Printf("\nreceived %s — shutting down gracefully\n", sig)
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
		os.Exit(1)
	}
}

func healthHandler(env string, cfg map[string]string) http.HandlerFunc {
	logLevel := cfg["logging.level"]
	if logLevel == "" {
		logLevel = "info"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body := fmt.Sprintf(
			"module=%s\nlanguage=Go\nruntime=%s\nenv=%s\nlog.level=%s\nfeatures=goroutines,channels,defer,graceful-shutdown,net/http\n",
			moduleName, runtime.Version(), env, logLevel,
		)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

// loadConfig reads application-{env}.yaml from MARCHE_CONFIG_DIR (or "configuration/")
// and parses two-level YAML into dot-notation keys. Falls back to hardcoded defaults
// if the file is absent or unreadable.
func loadConfig(env string) map[string]string {
	cfg := map[string]string{
		"server.port":   defaultPort,
		"logging.level": "info",
	}

	dir := envOrDefault("MARCHE_CONFIG_DIR", defaultDir)
	f, err := os.Open(dir + "/application-" + env + ".yaml")
	if err != nil {
		return cfg
	}
	defer f.Close()

	var section string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, ":") {
				section = strings.TrimSuffix(trimmed, ":")
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(trimmed, ": "); idx >= 0 && section != "" {
			cfg[section+"."+trimmed[:idx]] = trimmed[idx+2:]
		}
	}
	return cfg
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
