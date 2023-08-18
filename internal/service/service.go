package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/models"
	"go.uber.org/zap"
)

type TrainService struct {
	logger       *zap.Logger
	trainClient  *client.TrainClient
	database     *database.DatabaseClient
	constantList map[string]string
	config       config.Config
}

func NewTrainService(logger *zap.Logger, trainClient *client.TrainClient, constantsClient *constants.ConstantsClient, config config.Config, database *database.DatabaseClient) *TrainService {
	constantsList, err := constantsClient.FetchConstants()
	if err != nil {
		logger.Fatal("Failed to fetch constants", zap.Error(err))
	}
	return &TrainService{
		logger:       logger,
		trainClient:  trainClient,
		constantList: constantsList,
		config:       config,
		database:     database,
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
