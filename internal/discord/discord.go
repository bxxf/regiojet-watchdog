package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/errors"
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

func (s *DiscordService) NotifyDiscord(fields []map[string]interface{}, routeDetails models.RouteDetails, departureDate, webhookURL string) (int, error) {

	formattedDepartureDate, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	formattedArrivalDate, _ := time.Parse(time.RFC3339, routeDetails.ArrivalTime)

	formattedDepartureString := formattedDepartureDate.Format("15:04")
	formattedArrivalString := formattedArrivalDate.Format("15:04")

	payload := map[string]interface{}{
		"content": "",
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("Tickets available (%s -> %s) - %s -> %s [%s]", routeDetails.DepartureCityName, routeDetails.ArrivalCityName, formattedDepartureString, formattedArrivalString, departureDate),
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
		return errors.NotifyServiceJsonError, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return errors.NotifyServiceHttpError, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return errors.NotifyServiceHttpError, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.NotifyServiceHttpStatusError, &errors.NotifyServiceStatusError{Status: resp.StatusCode}
	}

	return errors.NotifyServiceOk, nil
}

func (s *DiscordService) NotifyDiscordAlternatives(alternatives []map[string]interface{}, routeInfo map[string]string, webhookURL string) (int, error) {
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
		return errors.NotifyServiceJsonError, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return errors.NotifyServiceHttpError, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return errors.NotifyServiceHttpError, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.NotifyServiceHttpStatusError, &errors.NotifyServiceStatusError{Status: resp.StatusCode}
	}

	return errors.NotifyServiceOk, nil
}
