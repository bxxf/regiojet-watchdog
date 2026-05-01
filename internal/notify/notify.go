package notify

import (
	"bytes"
	"encoding/json"
	errs "errors"
	"fmt"
	"net/http"
	"time"

	discordpkg "github.com/bxxf/regiojet-watchdog/internal/discord"
	"github.com/bxxf/regiojet-watchdog/internal/errors"
	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

type NotifyService struct {
	discordService *discordpkg.DiscordService
	logger         *zap.Logger
}

func NewNotifyService(logger *zap.Logger, discordService *discordpkg.DiscordService) *NotifyService {
	return &NotifyService{
		logger:         logger,
		discordService: discordService,
	}
}

func (n *NotifyService) Dispatch(freeSeatsDetails models.FreeSeatsResponse, routeDetails models.RouteDetails, routeDeparture, webhookType string, webhookURL string) {
	if routeDetails.FreeSeatsCount == 0 {
		return
	}

	departureTime, _ := time.Parse(time.RFC3339, routeDeparture)
	departureDate := departureTime.Format("02.01.2006")

	var seatCount map[int]int = map[int]int{}
	for _, section := range freeSeatsDetails {
		for _, vehicle := range section.Vehicles {
			seatCount[vehicle.VehicleNumber] += len(vehicle.FreeSeats)
		}
	}

	var fields []map[string]interface{} = []map[string]interface{}{}
	for vehicleNumber, count := range seatCount {
		if count > 0 {
			field := map[string]interface{}{
				"name":   fmt.Sprintf("Vehicle Number: %d", vehicleNumber),
				"value":  fmt.Sprintf("Number of Free Seats: %d", count),
				"inline": true,
			}
			fields = append(fields, field)
		}
	}

	var status = 0
	var err error
	if webhookType == "discord" {
		status, err = n.discordService.NotifyDiscord(fields, routeDetails, departureDate, webhookURL)
	} else {
		status, err = n.NotifySimple(fields, routeDetails, departureDate, webhookURL)
	}
	n.handleError(webhookType, status, err)
}

func (n *NotifyService) DispatchAlternative(allRoutes [][]map[string]string, webhookType string, webhookURL string) {
	var alternatives []map[string]interface{}

	var routeInfo map[string]string
	for _, route := range allRoutes {
		if len(route) == 0 {
			continue
		}
		var segmentsDescription string
		totalPrice := route[len(route)-1]["totalPrice"]

		routeInfo = route[0]

		if len(route) > 2 {
			routeInfo["to"] = route[len(route)-2]["to"]
		}

		for i, segment := range route {
			if i == len(route)-1 {
				break
			}

			var realInfoTo string
			if i == 0 && len(route) > 2 {
				realInfoTo = route[i+1]["from"]
			} else if len(route) < 3 {
				realInfoTo = routeInfo["to"]
			} else {
				realInfoTo = segment["to"]
			}
			segmentsDescription += fmt.Sprintf("**%s -> %s** (Departure: %s, Arrival: %s) \n *Free Seats: %s, Price: %s CZK*\n",
				segment["from"], realInfoTo, segment["departureTime"], segment["arrivalTime"], segment["freeSeats"], segment["price"])

		}

		alternative := map[string]interface{}{
			"name":   fmt.Sprintf("Alternative route with Total Price: %s CZK", totalPrice),
			"value":  segmentsDescription,
			"inline": false,
		}

		alternatives = append(alternatives, alternative)
	}

	var status = 0
	var err error
	if webhookType == "discord" {
		status, err = n.discordService.NotifyDiscordAlternatives(alternatives, routeInfo, webhookURL)
	} else {
		status, err = n.NotifyAlternatives(alternatives, routeInfo, webhookURL)
	}
	n.handleError(webhookType, status, err)
}

func (n *NotifyService) NotifySimple(fields []map[string]interface{}, routeDetails models.RouteDetails, departureDate, webhookURL string) (int, error) {
	payload := map[string]interface{}{
		"route_details":  routeDetails,
		"departure_date": departureDate,
		"fields":         fields,
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
	if resp.StatusCode != http.StatusOK {
		return errors.NotifyServiceHttpStatusError, &errors.NotifyServiceStatusError{Status: resp.StatusCode}
	}

	return errors.NotifyServiceOk, nil
}

func (n *NotifyService) NotifyAlternatives(alternatives []map[string]interface{}, routeInfo map[string]string, webhookURL string) (int, error) {
	payload := map[string]interface{}{
		"route_info": routeInfo,
		"fields":     alternatives,
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
	if resp.StatusCode != http.StatusOK {
		return errors.NotifyServiceHttpStatusError, &errors.NotifyServiceStatusError{Status: resp.StatusCode}
	}
	return errors.NotifyServiceOk, nil
}

func (n *NotifyService) handleError(webhookType string, code int, err error) {
	switch code {
	case errors.NotifyServiceJsonError:
		n.logger.Error("Failed to marshal "+webhookType+" JSON payload", zap.Error(err))
	case errors.NotifyServiceHttpError:
		n.logger.Error("Failed to send "+webhookType+" notification", zap.Error(err))
	case errors.NotifyServiceHttpStatusError:
		var statusErr *errors.NotifyServiceStatusError
		if errs.As(err, &statusErr) {
			n.logger.Error(
				"Notification to "+webhookType+" returned wrong status",
				zap.Int("status", statusErr.Status),
			)
			return
		}
		n.logger.Error("Notification to "+webhookType+" failed", zap.Error(err))
	}
}
