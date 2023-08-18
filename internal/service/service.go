package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

type TrainService struct {
	logger       *zap.Logger
	trainClient  *client.TrainClient
	constantList map[string]string
	config       config.Config
}

func NewTrainService(logger *zap.Logger, trainClient *client.TrainClient, constantsClient *constants.ConstantsClient, config config.Config) *TrainService {
	constantsList, err := constantsClient.FetchConstants()
	if err != nil {
		logger.Fatal("Failed to fetch constants", zap.Error(err))
	}
	return &TrainService{
		logger:       logger,
		trainClient:  trainClient,
		constantList: constantsList,
		config:       config,
	}
}

func (s *TrainService) NotifyDiscord(freeSeatsDetails models.FreeSeatsResponse, routeDetails models.RouteDetails, webhookURL string) {
	if routeDetails.FreeSeatsCount == 0 {
		return
	}
	var fields []map[string]interface{} = []map[string]interface{}{}
	for _, section := range freeSeatsDetails {
		for _, vehicle := range section.Vehicles {
			if len(vehicle.FreeSeats) > 0 {
				field := map[string]interface{}{
					"name":   fmt.Sprintf("Vehicle Number: %d", vehicle.VehicleNumber),
					"value":  fmt.Sprintf("Number of Free Seats: %d", len(vehicle.FreeSeats)),
					"inline": true,
				}
				fields = append(fields, field)
			}
		}
	}

	formattedDepartureDate, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	formattedArrivalDate, _ := time.Parse(time.RFC3339, routeDetails.ArrivalTime)

	formattedDepartureString := formattedDepartureDate.Format("15:04")
	formattedArrivalString := formattedArrivalDate.Format("15:04")

	payload := map[string]interface{}{
		"content": "",
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("New tickets available (%s -> %s) - %s -> %s", routeDetails.DepartureCityName, routeDetails.ArrivalCityName, formattedDepartureString, formattedArrivalString),
				"description": fmt.Sprintf("Travel Time: %s, Free seats count: %d", routeDetails.TravelTime, routeDetails.FreeSeatsCount),
				"color":       3447003,
				"fields":      fields,
				"footer": map[string]interface{}{
					"text": fmt.Sprintf("Price From: %d%s, Price To: %d%s", int(routeDetails.PriceFrom), "CZK", int(routeDetails.PriceTo), "CZK"),
				},
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		s.logger.Fatal("Failed to marshal JSON payload", zap.Error(err))
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(jsonPayload))
	if err != nil {
		s.logger.Fatal("Failed to send Discord notification", zap.Error(err))
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			s.logger.Error("Failed to send Discord notification", zap.Int("status", resp.StatusCode))
		}
	}
}

func (s *TrainService) GetRouteDetails(routeID int, fromStationID, toStationID string) (*models.RouteDetails, error) {
	url := "https://brn-ybus-pubapi.sa.cz/restapi/routes/" + strconv.Itoa(routeID) +
		"/simple?fromStationId=" + fromStationID + "&toStationId=" + toStationID

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Currency", "CZK")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResponse models.RouteDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, err
	}

	if len(apiResponse.Sections) == 0 {
		return nil, fmt.Errorf("no sections available in the response")
	}

	return &models.RouteDetails{
		PriceFrom:         apiResponse.PriceFrom,
		PriceTo:           apiResponse.PriceTo,
		FreeSeatsCount:    apiResponse.FreeSeatsCount,
		DepartureCityName: apiResponse.DepartureCityName,
		ArrivalCityName:   apiResponse.ArrivalCityName,
		TravelTime:        apiResponse.Sections[0].TravelTime,
		DepartureTime:     apiResponse.DepartureTime,
		ArrivalTime:       apiResponse.ArrivalTime,
	}, nil
}

func (s *TrainService) NotifyDiscordAlternatives(allRoutes [][]map[string]string, webhookURL string) {
	var alternatives []map[string]interface{}

	for _, route := range allRoutes {
		var segmentsDescription string
		totalPrice := route[len(route)-1]["totalPrice"]

		for i, segment := range route {
			if i == len(route)-1 {
				break
			}
			segmentsDescription += fmt.Sprintf("From %s to %s (Departure: %s, Arrival: %s, Free Seats: %s, Price: %s CZK)\n",
				segment["from"], segment["to"], segment["departureTime"], segment["arrivalTime"], segment["freeSeats"], segment["price"])
		}

		alternative := map[string]interface{}{
			"name":   fmt.Sprintf("Alternative route with Total Price: %s CZK", totalPrice),
			"value":  segmentsDescription,
			"inline": false,
		}

		alternatives = append(alternatives, alternative)
	}

	payload := map[string]interface{}{
		"content": "",
		"embeds": []map[string]interface{}{
			{
				"title":  "Alternative routes found (from least to most seat changes)",
				"color":  3447003,
				"fields": alternatives,
				"footer": map[string]interface{}{
					"text": fmt.Sprintf("Last updated at %s", time.Now().Format("15:04:05")),
				},
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		s.logger.Fatal("Failed to marshal JSON payload", zap.Error(err))
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(jsonPayload))
	if err != nil {
		s.logger.Fatal("Failed to send Discord notification", zap.Error(err))
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			s.logger.Error("Failed to send Discord notification", zap.Int("status", resp.StatusCode))
		}
	}
}
