package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "pitchbin.db", "SQLite database path")
	baseURL := flag.String("base-url", "", "public base URL (e.g. https://pitchbin.io)")
	powBits := flag.Int("pow-bits", 18, "proof-of-work difficulty in leading zero bits")
	annotationPowBits := flag.Int("annotation-pow-bits", 16, "proof-of-work difficulty for annotations")
	maxSize := flag.Int("max-size", 512000, "max markdown size in bytes")
	rateLimit := flag.Int("rate-limit", 5, "submissions per minute per IP")
	trustedProxy := flag.String("trusted-proxy", "", "IP of trusted reverse proxy (enables X-Forwarded-For)")
	devMode := flag.Bool("dev", false, "development mode (re-render markdown on each view)")
	flag.Parse()

	if *baseURL == "" {
		host, port := *addr, ""
		if i := strings.LastIndex(*addr, ":"); i >= 0 {
			host, port = (*addr)[:i], (*addr)[i:]
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		*baseURL = fmt.Sprintf("http://%s%s", host, port)
	}

	store, err := NewStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	renderer := NewRenderer()

	srv := NewServer(store, renderer, *baseURL, *powBits, *annotationPowBits, *maxSize, *rateLimit, *trustedProxy, *devMode)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go RunCleanup(ctx, store, srv.limiter, 10*time.Minute)

	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s", *addr)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
