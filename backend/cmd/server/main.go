package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/config"
	"auction-platform/backend/internal/database"
	"auction-platform/backend/internal/server"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // .env is optional; real environment variables win
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.MigrateOnStart {
		if err := database.Migrate(cfg.DatabaseURL); err != nil {
			return err
		}
		slog.Info("migrations applied")
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	st := store.New(pool)

	var line auth.LineClient
	if cfg.MockLineAuth {
		slog.Warn("MOCK_LINE_AUTH is enabled: anyone can log in as any mock user")
		line = auth.MockLineClient{FrontendURL: cfg.FrontendURL}
	} else {
		line = auth.NewHTTPLineClient(cfg.LineChannelID, cfg.LineChannelSecret, cfg.LineRedirectURI)
	}
	now := func() time.Time { return time.Now().UTC() }
	authSvc := auth.NewService(st, line, cfg.AdminLineUserIDs, cfg.SessionTTL, now)
	files := &storage.Local{Dir: cfg.UploadDir, BaseURL: "/uploads"}
	auctionSvc := auction.NewService(st, files, now)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: server.NewRouter(server.Deps{
			Config: cfg, Store: st, Auth: authSvc, Line: line, Auction: auctionSvc, Uploads: files,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // image uploads from mobile networks
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
