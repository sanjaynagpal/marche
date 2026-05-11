package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const usage = `health-probe-go — Marche service health checker

Usage:
  health-probe-go <url> [url ...]

Each URL is queried with a 5-second timeout. The response body is printed for
every endpoint. Exits 0 only when all endpoints return HTTP 200.

Examples:
  health-probe-go http://localhost:8080/health
  health-probe-go http://localhost:8080/health http://localhost:8081/health
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	allOK := true

	for _, url := range os.Args[1:] {
		if !probe(client, url) {
			allOK = false
		}
	}

	if !allOK {
		os.Exit(1)
	}
}

func probe(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL  %s\n      %v\n\n", url, err)
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("OK    %s\n%s\n", url, string(body))
		return true
	}

	fmt.Fprintf(os.Stderr, "FAIL  %s  (HTTP %d)\n%s\n\n", url, resp.StatusCode, string(body))
	return false
}
