package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/andreipimenov/golang-training-2021/internal/config"
	"github.com/andreipimenov/golang-training-2021/internal/handler"
	"github.com/andreipimenov/golang-training-2021/internal/repository"
	"github.com/andreipimenov/golang-training-2021/internal/service"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, err := config.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("Configuration error")
	}

	r := chi.NewRouter()

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(cfg.DBConnString))
	if err != nil {
		logger.Fatal().Err(err).Msg("DB initializing error")
	}
	defer client.Disconnect(context.TODO())

	err = client.Ping(context.TODO(), readpref.Primary())
	if err != nil {
		logger.Fatal().Err(err).Msg("DB pinging error")
	}

	db := client.Database("backend").Collection("prices")

	dbRepo := repository.NewDB(db)
	service := service.New(&logger, dbRepo, cfg.ExternalAPIToken)
	h := handler.New(&logger, service)

	r.Route("/", func(r chi.Router) {
		r.Use(middleware.RequestLogger(&handler.LogFormatter{Logger: &logger}))
		r.Use(middleware.Recoverer)
		r.Method(http.MethodGet, handler.Path, h)
	})

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)

	go func() {
		logger.Info().Msgf("Server is listening on :%d", cfg.Port)
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Server error")
		}
	}()

	<-shutdown

	logger.Info().Msg("Shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer func() {
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server shutdown error")
	}

	logger.Info().Msg("Server stopped gracefully")
}
