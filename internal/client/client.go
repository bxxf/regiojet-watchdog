package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

type TrainClient struct {
	logger *zap.Logger
	client *http.Client
}

type Route struct {
	ID            string  `json:"id"`
	DepartureTime string  `json:"departureTime"`
	ArrivalTime   string  `json:"arrivalTime"`
	PriceFrom     float64 `json:"priceFrom"`
	PriceTo       float64 `json:"priceTo"`
	FreeSeats     int     `json:"freeSeatsCount"`
}

func NewTrainClient(logger *zap.Logger) *TrainClient {
	return &TrainClient{
		logger: logger,
		client: &http.Client{},
	}
}

func (c *TrainClient) FetchRoutes(stationFromID, stationToID, departureDate, currency string) ([]Route, error) {
	parsedDepartureDate, err := time.Parse("02.01.2006", departureDate)
	if err != nil {
		return nil, err
	}
	formattedDepartureDate := parsedDepartureDate.Format("2006-01-02")
	url := fmt.Sprintf(
		"https://brn-ybus-pubapi.sa.cz/restapi/routes/search/simple?fromLocationId=%s&fromLocationType=STATION&toLocationId=%s&toLocationType=STATION&departureDate=%s",
		stationFromID,
		stationToID,
		formattedDepartureDate,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Currency", currency)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var responseJson models.Response
	if err := json.NewDecoder(resp.Body).Decode(&responseJson); err != nil {
		return nil, err
	}

	var routes []Route
	for _, ticket := range responseJson.Routes {
		vehicleType := ticket.VehicleTypes[0]
		containsBus := false
		if vehicleType == "BUS" {
			containsBus = true
			break
		}

		if containsBus {
			continue
		}

		departureTime, err := time.Parse(time.RFC3339, ticket.DepartureTime)
		if err != nil {
			c.logger.Fatal("Failed to parse departure time", zap.Error(err))
		}

		if departureTime.Format("02.01.2006") != departureDate {
			continue
		}

		if ticket.TransfersCount > 1 {
			continue
		}

		departureString := departureTime.Format("15:04")

		arrivalTime, err := time.Parse(time.RFC3339, ticket.ArrivalTime)
		if err != nil {
			c.logger.Fatal("Failed to parse arrival time", zap.Error(err))
		}

		arrivalString := arrivalTime.Format("15:04")

		routes = append(routes, Route{
			ID:            ticket.ID,
			DepartureTime: departureString,
			ArrivalTime:   arrivalString,
			PriceFrom:     ticket.PriceFrom,
			PriceTo:       ticket.PriceTo,
			FreeSeats:     ticket.FreeSeatsCount,
		})

	}

	return routes, nil
}

func (c *TrainClient) fetchFreeSeats(routeId int, seatclass, stationFromID, stationToID string) (*models.FreeSeatsResponse, *models.FreeSeatsError) {
	url := fmt.Sprintf("https://brn-ybus-pubapi.sa.cz/restapi/routes/%d/freeSeats", routeId)

	fromStationId, _ := strconv.Atoi(stationFromID)
	toStationId, _ := strconv.Atoi(stationToID)

	body := map[string]interface{}{

		"sections": []map[string]int{
			{
				"sectionId":     routeId,
				"fromStationId": fromStationId,
				"toStationId":   toStationId,
			},
		},
		"tariffs":   []string{"REGULAR"},
		"seatClass": seatclass,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, &models.FreeSeatsError{Message: err.Error()}
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &models.FreeSeatsError{Message: "Failed to fetch free seats"}
	}
	defer resp.Body.Close()

	var freeSeatsResponse models.FreeSeatsResponse
	var freeSeatsError models.FreeSeatsError

	jsonBodyDebug, _ := json.Marshal(body)
	c.logger.Info("Free seats response", zap.String("response", string(jsonBodyDebug)))

	err = json.NewDecoder(resp.Body).Decode(&freeSeatsResponse)
	if err != nil {
		err = json.NewDecoder(resp.Body).Decode(&freeSeatsError)
		if err != nil {
			return nil, &models.FreeSeatsError{Message: "Failed to fetch free seats"}
		}
		return nil, &freeSeatsError
	}
	return &freeSeatsResponse, nil

}

func (c *TrainClient) GetFreeSeats(routeID int, stationFromID, stationToID string) (models.FreeSeatsResponse, error) {
	freeSeatsResponse0, error := c.fetchFreeSeats(routeID, "C0", stationFromID, stationToID)
	freeSeatsResponse2, error := c.fetchFreeSeats(routeID, "C2", stationFromID, stationToID)

	if error != nil {
		c.logger.Info("Failed to fetch data", zap.String("error", error.Message))
		return nil, fmt.Errorf(error.Message)
	}

	freeSeatsResponse := append(*freeSeatsResponse0, *freeSeatsResponse2...)

	return freeSeatsResponse, nil

}
