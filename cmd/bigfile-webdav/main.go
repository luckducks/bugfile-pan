package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bugfile-pan/internal/bigfile"
	"bugfile-pan/internal/catalog"
	"bugfile-pan/internal/config"
	"bugfile-pan/internal/dav"
	"bugfile-pan/internal/upload"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}
	client := bigfile.NewClient(cfg.BaseURL, cfg.HTTPTimeout)
	store := &catalog.Store{}
	gateway, err := dav.NewGateway(cfg.ShareHash, cfg.Prefix, client, store)
	if err != nil {
		logger.Fatalf("create gateway: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Refresh(ctx); err != nil {
		logger.Fatalf("initial refresh failed: %v", err)
	}
	logger.Printf("loaded BigFile share %s", cfg.ShareHash)

	if cfg.RefreshInterval > 0 {
		go refreshLoop(ctx, logger, gateway, cfg.RefreshInterval)
	}

	uploadProxy, err := upload.NewProxy(cfg.UploadURL, &http.Client{
		Timeout: cfg.UploadTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
	if err != nil {
		logger.Fatalf("create upload proxy: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/upload", uploadProxy)
	if info, statErr := os.Stat(cfg.WebDir); statErr == nil && info.IsDir() {
		files := http.FileServer(http.Dir(cfg.WebDir))
		mux.Handle("/ui/", http.StripPrefix("/ui/", files))
		mux.Handle("/ui", http.RedirectHandler("/ui/", http.StatusPermanentRedirect))
		logger.Printf("serving upload UI at /ui/")
	} else {
		logger.Printf("upload UI disabled: WEB_DIR %q is unavailable", cfg.WebDir)
	}
	mux.Handle("/", gateway)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s%s", cfg.ListenAddr, cfg.Prefix)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("serve: %v", err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}

func refreshLoop(ctx context.Context, logger *log.Logger, gateway *dav.Gateway, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := gateway.Refresh(ctx); err != nil {
				logger.Printf("refresh failed: %v", err)
			}
		}
	}
}
