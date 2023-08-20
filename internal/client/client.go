package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

const baseURL = "https://brn-ybus-pubapi.sa.cz/restapi"

type TrainClient struct {
	logger *zap.Logger
	client *http.Client
}

func NewTrainClient(logger *zap.Logger) *TrainClient {
	return &TrainClient{
		logger: logger,
		client: &http.Client{},
	}
}

func (c *TrainClient) makeAPIRequest(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, baseURL+url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return c.client.Do(req)
}

func (c *TrainClient) FetchRoutes(stationFromID, stationToID, departureDate, currency string) ([]models.Route, error) {
	parsedDepartureDate, err := time.Parse("02.01.2006", departureDate)
	if err != nil {
		return nil, err
	}
	formattedDepartureDate := parsedDepartureDate.Format("2006-01-02")
	urlPath := fmt.Sprintf("/routes/search/simple?fromLocationId=%s&fromLocationType=STATION&toLocationId=%s&toLocationType=STATION&departureDate=%s",
		stationFromID,
		stationToID,
		formattedDepartureDate,
	)

	headers := map[string]string{
		"X-Currency": currency,
	}
	resp, err := c.makeAPIRequest("GET", urlPath, nil, headers)
	if err != nil {
		fmt.Printf("error in fetching routes %+v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	var responseJson models.Response
	if err := json.NewDecoder(resp.Body).Decode(&responseJson); err != nil {
		return nil, err
	}

	var routes []models.Route
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

		routes = append(routes, models.Route{
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
	urlPath := fmt.Sprintf("/routes/%d/freeSeats", routeId)

	fromStationId, err := strconv.Atoi(stationFromID)
	if err != nil {
		return nil, &models.FreeSeatsError{Message: "Invalid stationFromID"}
	}

	toStationId, err := strconv.Atoi(stationToID)
	if err != nil {
		return nil, &models.FreeSeatsError{Message: "Invalid stationToID"}
	}

	bodyMap := map[string]interface{}{
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

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, &models.FreeSeatsError{Message: err.Error()}
	}

	resp, err := c.makeAPIRequest("POST", urlPath, body, map[string]string{"Content-Type": "application/json"})
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
	var combinedFreeSeatsResponse models.FreeSeatsResponse
	seatClasses := []string{"C0", "C1", "C2"}

	for _, seatClass := range seatClasses {
		resp, err := c.fetchFreeSeats(routeID, seatClass, stationFromID, stationToID)
		if err != nil {
			c.logger.Error("Failed to fetch data", zap.String("error", err.Message))
			return nil, errors.New(err.Message)
		}
		if resp != nil {
			combinedFreeSeatsResponse = append(combinedFreeSeatsResponse, *resp...)
		}
	}

	if len(combinedFreeSeatsResponse) == 0 {
		return nil, errors.New("Failed to fetch data")
	}
	return combinedFreeSeatsResponse, nil
}

func (c *TrainClient) FetchStops(routeID string) (*models.TimetableResponse, error) {
	urlPath := fmt.Sprintf("/consts/timetables/%s", routeID)

	resp, err := c.makeAPIRequest("GET", urlPath, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var timetable models.TimetableResponse
	if err := json.NewDecoder(resp.Body).Decode(&timetable); err != nil {
		fmt.Printf("error in fetching stops %+v\n", err)
	}

	if len(timetable.Stations) == 0 {
		return nil, fmt.Errorf("no timetable available in the response")
	}
	return &timetable, nil
}

func (c *TrainClient) GetRouteDetails(routeID int, fromStationID, toStationID string) (*models.RouteDetails, error) {
	urlPath := fmt.Sprintf("/routes/%d/simple?fromStationId=%s&toStationId=%s", routeID, fromStationID, toStationID)

	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Currency", "CZK")

	resp, err := c.makeAPIRequest("GET", urlPath, nil, map[string]string{"X-Currency": "CZK"})
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
