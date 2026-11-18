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
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/siravitdell/user-management-api/internal/adapter/inbound/grpcapi"
	"github.com/siravitdell/user-management-api/internal/adapter/inbound/httpapi"
	"github.com/siravitdell/user-management-api/internal/adapter/inbound/scheduler"
	"github.com/siravitdell/user-management-api/internal/adapter/outbound/mongodb"
	"github.com/siravitdell/user-management-api/internal/adapter/outbound/security"
	"github.com/siravitdell/user-management-api/internal/config"
	"github.com/siravitdell/user-management-api/internal/core/service"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := mongodb.Connect(connectCtx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Error("mongo disconnect", "error", err)
		}
	}()
	repo, err := mongodb.NewUserRepository(connectCtx, client.Database(cfg.MongoDatabase))
	if err != nil {
		return err
	}
	logger.Info("connected to MongoDB", "database", cfg.MongoDatabase)

	hasher, err := security.NewBcryptHasher(bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tokens, err := security.NewJWTManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	if err != nil {
		return err
	}

	users := service.NewUserService(repo, hasher)
	auth, err := service.NewAuthService(repo, hasher, tokens)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(users, auth, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	grpcServer := grpcapi.NewGRPCServer(users, auth, logger)
	reporter := scheduler.NewUserCountReporter(users, cfg.UserCountInterval, logger)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			return fmt.Errorf("grpc listen: %w", err)
		}
		logger.Info("gRPC server listening", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		reporter.Run(gctx)
		return nil
	})

	g.Go(func() error {
		<-gctx.Done()
		logger.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("http shutdown", "error", err)
		}
		stopGRPC(shutdownCtx, grpcServer)
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

func stopGRPC(ctx context.Context, s *grpc.Server) {
	done := make(chan struct{})
	go func() { s.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		s.Stop()
	}
}
