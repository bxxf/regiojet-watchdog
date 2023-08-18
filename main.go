package main

import (
	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/logger"
	"github.com/bxxf/regiojet-watchdog/internal/segmentation"
	"github.com/bxxf/regiojet-watchdog/internal/server"
	"github.com/bxxf/regiojet-watchdog/internal/service"
	"go.uber.org/fx"
)

func main() {
	app := fx.New(
		fx.Provide(
			config.LoadConfig,
			client.NewTrainClient,
			service.NewTrainService,
			logger.NewLogger,
			constants.NewConstantsClient,
			segmentation.NewSegmentationService,
			server.NewServer,
			database.NewDatabaseClient,
		),
		fx.Invoke(constants.RegisterConstantsHooks, server.RegisterServerHooks),
	)

	app.Run()
}
