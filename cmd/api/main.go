package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/EvKrpv/payment-system/internal/api"
	"github.com/EvKrpv/payment-system/internal/config"
	"github.com/EvKrpv/payment-system/internal/idempotency"
	"github.com/EvKrpv/payment-system/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Загружаем .env
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("⚠️  Failed to get current directory")
		os.Exit(1)
	}

	envPath := filepath.Join(dir, ".env")
	if err := godotenv.Load(envPath); err != nil {
		fmt.Println("⚠️  No .env file found, using system environment variables")
	} else {
		fmt.Println("✅ .env file loaded successfully")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		fmt.Println("   Please check that all required environment variables are set in .env file")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("starting payment-api", "port", cfg.ServerPort)

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to PostgreSQL")

	// Создаём репозитории и сервисы
	paymentRepo := repository.NewPaymentRepository(pool)

	idempotencySvc := idempotency.NewIdempotencyService(pool)
	idempotencySvc.SetLogger(logger)

	paymentHandlers := api.NewPaymentHandlers(paymentRepo, idempotencySvc, logger)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"payment-api"}`))
	})

	mux.HandleFunc("GET /db-check", func(w http.ResponseWriter, r *http.Request) {
		var result int
		err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&result)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status":"error","message":"database connection failed"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","message":"database is healthy"}`))
	})

	mux.HandleFunc("POST /api/v1/payments", paymentHandlers.CreatePayment)
	mux.HandleFunc("GET /api/v1/payments/", paymentHandlers.GetPayment)

	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := api.RecoveryMiddleware(logger)(
		api.LoggingMiddleware(logger)(
			api.RequestIDMiddleware(mux),
		),
	)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("server started", "port", cfg.ServerPort)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
