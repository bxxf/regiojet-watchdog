package main

import (
	"github.com/bxxf/regiojet-watchdog/internal/checker"
	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/discord"
	"github.com/bxxf/regiojet-watchdog/internal/logger"
	"github.com/bxxf/regiojet-watchdog/internal/segmentation"
	"github.com/bxxf/regiojet-watchdog/internal/server"
	"go.uber.org/fx"
)

func main() {
	app := fx.New(
		fx.Provide(
			config.LoadConfig,
			client.NewTrainClient,
			logger.NewLogger,
			constants.NewConstantsClient,
			checker.NewChecker,
			segmentation.NewSegmentationService,
			server.NewServer,
			discord.NewDiscordService,
			database.NewDatabaseClient,
		),
		fx.Invoke(constants.RegisterConstantsHooks, server.RegisterServerHooks, checker.RegisterCheckerHooks),
	)

	app.Run()
}
