package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sdlc-coordinator/internal/app"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/logging"
	"sdlc-coordinator/internal/store"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	bootstrap := config.LoadBootstrap()
	legacy := config.Load()

	logger, err := logging.New(bootstrap.LogLevel, bootstrap.LogDir)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	db, err := store.Open(bootstrap.DatabaseDSN)
	if err != nil {
		logger.Fatal("open database failed", zap.Error(err))
	}

	configurationRepository := configuration.NewRepository(db)
	if err := store.MigrateGuarded(context.Background(), db, "configuration", configurationRepository.Migrate); err != nil {
		logger.Fatal("migrate runtime configuration failed", zap.Error(err))
	}
	resolution := configuration.NewResolver(configurationRepository).Resolve(context.Background(), bootstrap, legacy)
	configurationService := configuration.NewService(configurationRepository, resolution)
	shutdownSignal := configuration.NewShutdownSignal()
	application, err := buildApplication(bootstrap, resolution, configurationService, shutdownSignal, logger, db)
	if err != nil {
		logger.Fatal("initialize application failed", zap.Error(err))
	}
	logger.Info("coordinator startup mode resolved",
		zap.String("mode", string(resolution.Mode)),
		zap.Uint64("configuration_revision", configurationService.Status().Revision),
	)

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalContext)
	defer cancel()
	go func() {
		select {
		case <-shutdownSignal.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	go configuration.WatchForNewerRevision(ctx, configurationRepository, configurationService.Status().Revision, shutdownSignal, time.Second, func(err error) {
		logger.Warn("check runtime configuration revision failed", zap.Error(err))
	})

	if err := application.Run(ctx); err != nil {
		logger.Fatal("application stopped with error", zap.Error(err))
	}
}

func buildApplication(
	bootstrap config.BootstrapConfig,
	resolution configuration.Resolution,
	configurationService *configuration.Service,
	shutdown configuration.ShutdownRequester,
	logger *zap.Logger,
	db *gorm.DB,
) (*app.App, error) {
	switch resolution.Mode {
	case configuration.ModeReady:
		return app.NewReady(resolution.EffectiveConfig, logger, db, configurationService, shutdown)
	case configuration.ModeSetupPending, configuration.ModeConfigurationError:
		return app.NewBootstrap(bootstrap, logger, db, configurationService, shutdown, resolution.Mode)
	default:
		return nil, fmt.Errorf("unsupported coordinator startup mode %q", resolution.Mode)
	}
}
