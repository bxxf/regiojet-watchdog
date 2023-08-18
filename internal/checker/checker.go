package checker

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/discord"
	"github.com/bxxf/regiojet-watchdog/internal/segmentation"
	"github.com/bxxf/regiojet-watchdog/internal/service"
	"go.uber.org/fx"
)

type Checker struct {
	trainService        *service.TrainService
	discordService      *discord.DiscordService
	trainClient         *client.TrainClient
	database            *database.DatabaseClient
	segmentationService *segmentation.SegmentationService
}

func NewChecker(trainService *service.TrainService, database *database.DatabaseClient, segmentationService *segmentation.SegmentationService, client *client.TrainClient, discordService *discord.DiscordService) *Checker {
	return &Checker{
		trainService:        trainService,
		trainClient:         client,
		database:            database,
		segmentationService: segmentationService,
		discordService:      discordService,
	}
}

func (s *Checker) periodicallyCheck() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			keys, err := s.database.RedisClient.Keys(context.Background(), "watchdog:*").Result()
			if err != nil {
				log.Println("Failed to fetch keys:", err)
				continue
			}

			for _, key := range keys {
				value, err := s.database.RedisClient.Get(context.Background(), key).Result()
				if err != nil {
					log.Println("Failed to fetch value for key", key, ":", err)
					continue
				}

				parts := strings.Split(value, ";;")
				if len(parts) != 4 {
					log.Println("Invalid value format for key", key)
					continue
				}
				webhookURL, stationFromID, stationToID, routeIDStr := parts[0], parts[1], parts[2], parts[3]

				routeID, err := strconv.Atoi(routeIDStr)
				if err != nil {
					log.Println("Failed to parse routeID:", err)
					continue
				}

				freeSeatsResponse, err := s.trainClient.GetFreeSeats(routeID, stationFromID, stationToID)
				routeDetails, err := s.trainService.GetRouteDetails(routeID, stationFromID, stationToID)
				if err != nil {
					log.Println("Failed to fetch route details:", err)
					continue
				}

				if routeDetails.FreeSeatsCount > 0 {
					s.discordService.NotifyDiscord(freeSeatsResponse, *routeDetails, webhookURL)
				} else {
					departureTime, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
					departureDate := departureTime.Format("02.01.2006")
					availableSegments, err := s.segmentationService.FindAvailableSegments(strconv.Itoa(routeID), stationFromID, stationToID, departureDate)
					if err != nil {
						log.Println("Failed to fetch available segments:", err)
						continue
					}
					if len(availableSegments) > 0 {
						s.discordService.NotifyDiscordAlternatives(availableSegments, webhookURL)
					}
				}
			}
		}
	}
}

func RegisterCheckerHooks(lc fx.Lifecycle, checker *Checker) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go checker.periodicallyCheck()
			return nil
		},
		OnStop: nil,
	})
}
