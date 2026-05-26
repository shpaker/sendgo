// Command sendgo is the sendgo HTTP/WS server. Single binary, everything in one
// process: HTTP API, WS-upload, static asset serving, optional cleanup runner.
//
// Configured via CLI flags or env variables (kong). See `sendgo --help`.
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
	"syscall"
	"time"

	"github.com/sendgo/sendgo/server/internal/adapter/cleanup"
	httpx "github.com/sendgo/sendgo/server/internal/adapter/http"
	"github.com/sendgo/sendgo/server/internal/adapter/meta"
	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/adapter/storage"
	"github.com/sendgo/sendgo/server/internal/config"
	cryptotokens "github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func main() {
	cfg := config.Load(os.Args[1:])

	// Logger + log-file SIGHUP-handler.
	lg, logWriter, err := observability.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	// SetDefault so middleware/handlers that fall back to slog.Default() via
	// FromContext also write to our stream.
	slog.SetDefault(lg)
	stopSighup := logWriter.WatchSIGHUP()
	defer stopSighup()
	defer func() { _ = logWriter.Close() }()

	lg.Info("sendgo starting", "version", config.Version, "commit", config.Commit)

	// Sentry (optional).
	sentryFlush, err := observability.InitSentry(cfg, lg)
	if err != nil {
		lg.Error("sentry init failed", "err", err)
		os.Exit(1)
	}
	defer sentryFlush()

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Adapters ---
	metaStore, err := meta.New(rootCtx, cfg, lg)
	if err != nil {
		lg.Error("meta store init failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = metaStore.Close() }()

	blobStore, err := storage.New(rootCtx, cfg, lg)
	if err != nil {
		lg.Error("blob storage init failed", "err", err)
		os.Exit(1)
	}

	tokens := cryptotokens.RandomTokens{}
	clk := port.RealClock{}

	// --- Use cases ---
	// The InitiateUpload use case stays pure — the base URL is resolved per-
	// request inside the HTTP adapter (see router.go's wiring of UploadWS).
	initiate := &usecase.InitiateUpload{
		Tokens:  tokens,
		MaxFile: cfg.MaxFileSize,
		MaxTTL:  cfg.MaxExpireSeconds,
		MaxDL:   cfg.MaxDownloads,
		Defaults: usecase.Defaults{
			ExpireSeconds: cfg.DefaultExpireSeconds,
			Downloads:     cfg.DefaultDownloads,
		},
	}
	// MaxSize is in encrypted bytes: the frontend pushes an encrypted stream, so
	// our io.LimitReader must account for ECE tags/header overhead.
	streamUpload := &usecase.StreamUpload{
		Blob: blobStore,
		Meta: metaStore,
		// The frontend streams an already-encrypted payload (ECE framing), so the
		// byte limit must account for all tag and header overhead.
		MaxSize: domain.EncryptedSize(cfg.MaxFileSize),
		Clock:   clk,
	}
	checkExists := &usecase.CheckExists{Meta: metaStore}
	getMeta := &usecase.GetMetadata{Meta: metaStore}
	streamDownload := &usecase.StreamDownload{Blob: blobStore, Meta: metaStore}
	deleteShare := &usecase.DeleteShare{Blob: blobStore, Meta: metaStore}
	setPassword := &usecase.SetPassword{Meta: metaStore}
	updateParams := &usecase.UpdateParams{Meta: metaStore, MaxDownloads: cfg.MaxDownloads}
	getInfo := &usecase.GetInfo{Meta: metaStore}

	// --- HTTP router ---
	router := httpx.NewRouter(httpx.Deps{
		Cfg:            cfg,
		Meta:           metaStore,
		Blob:           blobStore,
		Tokens:         tokens,
		Clock:          clk,
		InitiateUpload: initiate,
		StreamUpload:   streamUpload,
		CheckExists:    checkExists,
		GetMetadata:    getMeta,
		StreamDownload: streamDownload,
		DeleteShare:    deleteShare,
		SetPassword:    setPassword,
		UpdateParams:   updateParams,
		GetInfo:        getInfo,
	})

	// --- Cleanup runner (optional) ---
	if cfg.CleanupEnabled {
		runner := &cleanup.Runner{
			Meta:     metaStore,
			Blob:     blobStore,
			Clock:    clk,
			Log:      lg.With("component", "cleanup"),
			Interval: cfg.CleanupInterval,
			Batch:    cfg.CleanupBatchSize,
		}
		go runner.Run(rootCtx)
	}

	// --- HTTP server ---
	addr := net.JoinHostPort(cfg.IPAddress, strconv.Itoa(cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	// Start + graceful shutdown.
	errCh := make(chan error, 1)
	go func() {
		lg.Info("http server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-rootCtx.Done():
		lg.Info("shutdown signal received")
	case err := <-errCh:
		lg.Error("http server failed", "err", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Warn("http server shutdown error", "err", err)
	}
}
