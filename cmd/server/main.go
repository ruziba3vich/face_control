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

	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"

	"udevs/face_control/internal/api"
	"udevs/face_control/internal/config"
	"udevs/face_control/internal/device"
	"udevs/face_control/internal/hasdk"
	"udevs/face_control/internal/registration"
	"udevs/face_control/internal/storage"
	"udevs/face_control/internal/user"

	"github.com/go-chi/chi/v5"
)

func main() {
	_ = godotenv.Load()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := sqlx.Connect("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 30*time.Second)
	photos, err := storage.NewMinioStore(bootCtx, storage.MinioConfig{
		Endpoint:  cfg.MinioEndpoint,
		AccessKey: cfg.MinioAccessKey,
		SecretKey: cfg.MinioSecretKey,
		Bucket:    cfg.MinioBucket,
		UseSSL:    cfg.MinioUseSSL,
	})
	bootCancel()
	if err != nil {
		log.Error("photo store", "err", err)
		os.Exit(1)
	}

	// Default to the device's HTTP API on :8000 (vendor-documented JSON
	// command interface). Set HASDK_NOOP=1 to fall back to the no-op logger.
	var sdkClient hasdk.Client = hasdk.NewHTTPClient(log)
	if os.Getenv("HASDK_NOOP") == "1" {
		sdkClient = hasdk.NewNoopClient(log)
	}
	defer sdkClient.Close()

	devRepo := device.NewRepo(db)
	userRepo := user.NewRepo(db)
	regRepo := registration.NewRepo(db)
	regSvc := &registration.Service{
		Devices:       devRepo,
		Users:         userRepo,
		Registrations: regRepo,
		Photos:        photos,
		HASdk:         sdkClient,
	}

	h := &api.Handler{
		Devices:          devRepo,
		Users:            userRepo,
		Registrations:    regSvc,
		RegistrationRepo: regRepo,
		Photos:           photos,
		HASdk:            sdkClient,
		Log:              log,
	}

	root := chi.NewRouter()
	root.Use(middleware.RequestID)
	root.Use(middleware.RealIP)
	root.Use(middleware.Recoverer)
	root.Use(middleware.Timeout(60 * time.Second))
	root.Mount("/", h.Routes())

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
