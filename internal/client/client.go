package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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

type Stop struct {
	StationID int      `json:"stationId"`
	Index     int      `json:"index"`
	Departure string   `json:"departure"`
	Arrival   string   `json:"arrival"`
	Symbols   []string `json:"symbols"`
	Platform  string   `json:"platform"`
}

type TimetableResponse struct {
	ConnectionID int    `json:"connectionId"`
	FromCityName string `json:"fromCityName"`
	ToCityName   string `json:"toCityName"`
	Stations     []Stop `json:"stations"`
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
	freeSeatsResponse1, error := c.fetchFreeSeats(routeID, "C1", stationFromID, stationToID)
	freeSeatsResponse2, error := c.fetchFreeSeats(routeID, "C2", stationFromID, stationToID)

	if error != nil {
		c.logger.Info("Failed to fetch data", zap.String("error", error.Message))
		return nil, fmt.Errorf(error.Message)
	}
	if freeSeatsResponse0 == nil && freeSeatsResponse2 == nil && freeSeatsResponse1 == nil {
		return nil, fmt.Errorf("Failed to fetch data")
	}

	if freeSeatsResponse0 == nil {
		freeSeatsResponse0 = &models.FreeSeatsResponse{}
	}
	if freeSeatsResponse1 == nil {
		freeSeatsResponse1 = &models.FreeSeatsResponse{}
	}
	if freeSeatsResponse2 == nil {
		freeSeatsResponse2 = &models.FreeSeatsResponse{}
	}

	freeSeatsResponse := append(*freeSeatsResponse0, *freeSeatsResponse2...)
	freeSeatsResponse = append(freeSeatsResponse, *freeSeatsResponse1...)

	return freeSeatsResponse, nil

}

func (client *TrainClient) FetchStops(routeID string) (*TimetableResponse, error) {
	// Construct the URL for the request
	url := fmt.Sprintf("https://brn-ybus-pubapi.sa.cz/restapi/consts/timetables/%s", routeID)

	// Make the HTTP request
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Failed to fetch stops:", err)
		return nil, err
	}
	defer resp.Body.Close()

	var timetable TimetableResponse
	if err := json.NewDecoder(resp.Body).Decode(&timetable); err != nil {
		log.Println("Failed to decode JSON:", err)
		return nil, err
	}

	// Return the stops information
	return &timetable, nil
}
