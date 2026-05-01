package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	clientpkg "github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	databasepkg "github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/models"
	"github.com/bxxf/regiojet-watchdog/internal/notify"
	segmentationpkg "github.com/bxxf/regiojet-watchdog/internal/segmentation"
	"go.uber.org/fx"
)

type Checker struct {
	config              config.Config
	notifyService       *notify.NotifyService
	trainClient         *clientpkg.TrainClient
	database            *databasepkg.DatabaseClient
	segmentationService *segmentationpkg.SegmentationService
}

func NewChecker(config config.Config, database *databasepkg.DatabaseClient, segmentationService *segmentationpkg.SegmentationService, client *clientpkg.TrainClient, notifyService *notify.NotifyService) *Checker {
	return &Checker{
		config:              config,
		trainClient:         client,
		database:            database,
		segmentationService: segmentationService,
		notifyService:       notifyService,
	}
}

func (c *Checker) handleKey(key string) {
	value, err := c.database.RedisClient.Get(context.Background(), key).Result()
	if err != nil {
		log.Println("Failed to fetch value for key", key, ":", err)
		return
	}

	var w models.Webhook
	err = json.Unmarshal([]byte(value), &w)
	if err != nil {
		log.Println("Failed to parse value:", err)
		return
	}

	routeDetails, freeSeatsResponse, err := c.fetchRouteDetails(w.RouteID, w.StationFromID, w.StationToID)
	if err != nil {
		log.Println("Failed to fetch route details or free seats:", err)
	}

	if routeDetails != nil && routeDetails.FreeSeatsCount > 0 {
		if freeSeatsResponse != nil {
			c.notifyService.Dispatch(*freeSeatsResponse, *routeDetails, routeDetails.DepartureTime, w.WebhookType, w.WebhookURL)
			if w.CheckSegments {
				c.notifyAlternativeSegments(w.RouteID, w.StationFromID, w.StationToID, routeDetails.DepartureTime, w.WebhookType, w.WebhookURL)
			}
		} else {
			fmt.Printf("Free seats count is %d, but free seats response is nil\n", routeDetails.FreeSeatsCount)
		}
	} else if routeDetails != nil && w.CheckSegments {
		c.notifyAlternativeSegments(w.RouteID, w.StationFromID, w.StationToID, routeDetails.DepartureTime, w.WebhookType, w.WebhookURL)
	} else {
		fmt.Printf("Free seats count is 0, but route details are nil - %v\n", routeDetails)
	}
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

func (c *Checker) notifyAlternativeSegments(routeIDStr, stationFromID, stationToID, departureTimeStr, webhookType string, webhookURL string) {
	departureTime, _ := time.Parse(time.RFC3339, departureTimeStr)
	departureDate := departureTime.Format("02.01.2006")
	availableSegments, err := c.segmentationService.FindAvailableSegments(routeIDStr, stationFromID, stationToID, departureDate)
	if err != nil {
		log.Println("Failed to fetch available segments:", err)
		return
	}
	if len(availableSegments) > 0 {
		c.notifyService.DispatchAlternative(availableSegments, webhookType, webhookURL)
	}
}

func (c *Checker) periodicallyCheck() {
	if c.config.CheckIntervalMinutes <= 0 {
		return
	}

	ticker := time.NewTicker(time.Duration(c.config.CheckIntervalMinutes) * time.Minute)
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
	if checker.config.CheckIntervalMinutes <= 0 {
		return
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go checker.periodicallyCheck()
			return nil
		},
		OnStop: nil,
	})
}
