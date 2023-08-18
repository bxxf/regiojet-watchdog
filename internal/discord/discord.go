package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

type DiscordService struct {
	logger *zap.Logger
}

func NewDiscordService(logger *zap.Logger) *DiscordService {
	return &DiscordService{
		logger: logger,
	}
}

func (s *DiscordService) NotifyDiscord(freeSeatsDetails models.FreeSeatsResponse, routeDetails models.RouteDetails, webhookURL string) {
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

func (s *DiscordService) NotifyDiscordAlternatives(allRoutes [][]map[string]string, webhookURL string) {
	var alternatives []map[string]interface{}

	var routeInfo map[string]string
	for _, route := range allRoutes {
		var segmentsDescription string
		totalPrice := route[len(route)-1]["totalPrice"]

		routeInfo = route[0]
		routeInfo["to"] = route[len(route)-2]["to"]

		for i, segment := range route {
			if i == len(route)-1 {
				break
			}

			if i == 0 {
				segmentsDescription += fmt.Sprintf("**%s -> %s** (Departure: %s, Arrival: %s) \n *Free Seats: %s, Price: %s CZK*\n",
					segment["from"], route[i+1]["from"], segment["departureTime"], segment["arrivalTime"], segment["freeSeats"], segment["price"])
			} else {
				segmentsDescription += fmt.Sprintf("**%s -> %s** (Departure: %s, Arrival: %s) \n *Free Seats: %s, Price: %s CZK*\n",

					segment["from"], segment["to"], segment["departureTime"], segment["arrivalTime"], segment["freeSeats"], segment["price"])
			}

			fmt.Printf("from %s to %s", segment["from"], segment["to"])

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
				"title":  fmt.Sprintf("Alternative routes %s -> %s (%s)", routeInfo["from"], routeInfo["to"], routeInfo["departureDate"]),
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
