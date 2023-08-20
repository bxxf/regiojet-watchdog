package checker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	clientpkg "github.com/bxxf/regiojet-watchdog/internal/client"
	databasepkg "github.com/bxxf/regiojet-watchdog/internal/database"
	discordpkg "github.com/bxxf/regiojet-watchdog/internal/discord"
	"github.com/bxxf/regiojet-watchdog/internal/models"
	segmentationpkg "github.com/bxxf/regiojet-watchdog/internal/segmentation"
	"go.uber.org/fx"
)

type Checker struct {
	discordService      *discordpkg.DiscordService
	trainClient         *clientpkg.TrainClient
	database            *databasepkg.DatabaseClient
	segmentationService *segmentationpkg.SegmentationService
}

func NewChecker(database *databasepkg.DatabaseClient, segmentationService *segmentationpkg.SegmentationService, client *clientpkg.TrainClient, discordService *discordpkg.DiscordService) *Checker {
	return &Checker{
		trainClient:         client,
		database:            database,
		segmentationService: segmentationService,
		discordService:      discordService,
	}
}

func (c *Checker) handleKey(key string) {
	value, err := c.database.RedisClient.Get(context.Background(), key).Result()
	if err != nil {
		log.Println("Failed to fetch value for key", key, ":", err)
		return
	}

	webhookURL, stationFromID, stationToID, routeIDStr, err := c.parseValue(value)
	if err != nil {
		log.Println("Failed to parse value:", err)
		return
	}

	routeDetails, freeSeatsResponse, err := c.fetchRouteDetails(routeIDStr, stationFromID, stationToID)
	if err != nil {
		log.Println("Failed to fetch route details or free seats:", err)
	}

	if routeDetails != nil && routeDetails.FreeSeatsCount > 0 {
		if freeSeatsResponse != nil {
			c.discordService.NotifyDiscord(*freeSeatsResponse, *routeDetails, routeDetails.DepartureTime, webhookURL)
			c.notifyAlternativeSegments(routeIDStr, stationFromID, stationToID, routeDetails.DepartureTime, webhookURL)
		} else {
			fmt.Printf("Free seats count is %d, but free seats response is nil\n", routeDetails.FreeSeatsCount)
		}
	} else if routeDetails != nil {
		c.notifyAlternativeSegments(routeIDStr, stationFromID, stationToID, routeDetails.DepartureTime, webhookURL)
	} else {
		fmt.Printf("Free seats count is 0, but route details are nil - %v\n", routeDetails)
	}

}

func (c *Checker) parseValue(value string) (webhookURL, stationFromID, stationToID, routeIDStr string, err error) {
	parts := strings.Split(value, ";;")
	if len(parts) != 4 {
		return "", "", "", "", errors.New("Invalid value format")
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func (c *Checker) fetchRouteDetails(routeIDStr, stationFromID, stationToID string) (*models.RouteDetails, *models.FreeSeatsResponse, error) {
	routeID, err := strconv.Atoi(routeIDStr)
	if err != nil {
		return nil, nil, err
	}

	freeSeatsResponse, err := c.trainClient.GetFreeSeats(routeID, stationFromID, stationToID)
	routeDetails, err := c.trainClient.GetRouteDetails(routeID, stationFromID, stationToID)
	return routeDetails, &freeSeatsResponse, err
}

func (c *Checker) notifyAlternativeSegments(routeIDStr, stationFromID, stationToID, departureTimeStr, webhookURL string) {
	departureTime, _ := time.Parse(time.RFC3339, departureTimeStr)
	departureDate := departureTime.Format("02.01.2006")
	availableSegments, err := c.segmentationService.FindAvailableSegments(routeIDStr, stationFromID, stationToID, departureDate)
	if err != nil {
		log.Println("Failed to fetch available segments:", err)
		return
	}
	if len(availableSegments) > 0 {
		c.discordService.NotifyDiscordAlternatives(availableSegments, webhookURL)
	}
}

func (c *Checker) periodicallyCheck() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			keys, err := c.database.RedisClient.Keys(context.Background(), "watchdog:*").Result()
			if err != nil {
				log.Println("Failed to fetch keys:", err)
				continue
			}
			for _, key := range keys {
				c.handleKey(key)
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
