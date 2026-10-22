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

	"auction-platform/backend/internal/admin"
	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/config"
	"auction-platform/backend/internal/database"
	"auction-platform/backend/internal/ecpay"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/payment"
	"auction-platform/backend/internal/report"
	"auction-platform/backend/internal/review"
	"auction-platform/backend/internal/scheduler"
	"auction-platform/backend/internal/server"
	"auction-platform/backend/internal/settlement"
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

	var line auth.LineClient // nil when no LINE login is available
	switch {
	case cfg.MockLineAuth:
		slog.Warn("MOCK_LINE_AUTH is enabled: anyone can log in as any mock user")
		line = auth.MockLineClient{FrontendURL: cfg.FrontendURL}
	case cfg.LineEnabled():
		line = auth.NewHTTPLineClient(cfg.LineChannelID, cfg.LineChannelSecret, cfg.LineRedirectURI)
	default:
		slog.Warn("LINE Login is not configured")
	}
	if cfg.TestLoginPassword != "" {
		slog.Warn("TEST_LOGIN_PASSWORD is set: password-protected test accounts are enabled; remove it before real launch")
	}
	now := func() time.Time { return time.Now().UTC() }
	authSvc := auth.NewService(st, line, cfg.AdminLineUserIDs, cfg.SessionTTL, now)
	var files storage.Storage
	var localFiles *storage.Local
	if cfg.StorageBackend == "r2" {
		r2, err := storage.NewS3(storage.S3Config{
			Endpoint: cfg.R2Endpoint, AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
			Bucket: cfg.R2Bucket, PublicBaseURL: cfg.R2PublicBaseURL,
		})
		if err != nil {
			return err
		}
		files = r2
		slog.Info("image storage: R2", "bucket", cfg.R2Bucket, "public_base_url", cfg.R2PublicBaseURL)
	} else {
		localFiles = &storage.Local{Dir: cfg.UploadDir, BaseURL: "/uploads"}
		files = localFiles
		slog.Info("image storage: local disk", "dir", cfg.UploadDir)
	}

	var dispatcher notify.Dispatcher = notify.Noop{}
	var pusher *notify.Pusher
	if cfg.PushEnabled() {
		pusher = notify.NewPusher(st, notify.VAPIDSender(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject))
		dispatcher = pusher
	} else {
		slog.Warn("VAPID keys not set: Web Push disabled (in-app notifications still work)")
	}

	auctionSvc := auction.NewService(st, files, dispatcher, now)
	orderSvc := order.NewService(st, files, dispatcher, order.Options{
		AutoReceiveAfter:      cfg.OrderAutoReceiveAfter,
		AutoCompleteAfter:     cfg.OrderAutoCompleteAfter,
		ShippingInfoRetention: cfg.ShippingInfoRetention,
	}, now)

	var ecpayClient *ecpay.Client
	if cfg.ECPayEnabled() {
		ecpayClient = ecpay.New(cfg.ECPayMerchantID, cfg.ECPayHashKey, cfg.ECPayHashIV, cfg.ECPayBaseURL)
	} else {
		slog.Warn("ECPAY_MERCHANT_ID not set: payments disabled")
	}
	paymentSvc := payment.NewService(st, ecpayClient, dispatcher, payment.Config{
		PublicBaseURL: cfg.PublicBaseURL, AllowSimulated: cfg.ECPayAllowSimulated,
	}, now)
	settlementSvc := settlement.NewService(st, dispatcher, now)
	reviewSvc := review.NewService(st, dispatcher, now)
	reportSvc := report.NewService(st, now)
	adminSvc := admin.NewService(st, dispatcher, now)

	var schedulerDone chan struct{}
	if cfg.SchedulerEnabled {
		schedulerDone = make(chan struct{})
		go func() {
			defer close(schedulerDone)
			slog.Info("scheduler started", "interval", cfg.SchedulerInterval.String())
			scheduler.Run(ctx, cfg.SchedulerInterval,
				scheduler.Job{Name: "settlement", Run: settlementSvc.RunOnce},
				scheduler.Job{Name: "orders", Run: orderSvc.RunJobs},
			)
		}()
	}

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: server.NewRouter(server.Deps{
			Config: cfg, Store: st, Auth: authSvc, Line: line, Auction: auctionSvc, Order: orderSvc, Payment: paymentSvc,
			Review: reviewSvc, Report: reportSvc, Admin: adminSvc,
			Uploads: localFiles, Now: now,
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
	stop() // also stops the scheduler
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if schedulerDone != nil {
		<-schedulerDone
	}
	if pusher != nil {
		pusher.Wait(shutdownCtx) // let in-flight pushes finish
	}
	return err
}
