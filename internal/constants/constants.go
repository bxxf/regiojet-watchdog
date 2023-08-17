package constants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ConstantsClient struct {
	logger *zap.Logger
}

func NewConstantsClient(logger *zap.Logger) *ConstantsClient {
	return &ConstantsClient{
		logger: logger,
	}
}

type Country struct {
	Cities []City `json:"cities"`
}

type City struct {
	Stations []Station `json:"stations"`
}

type Station struct {
	ID           int64    `json:"id"`
	FullName     string   `json:"fullname"`
	StationTypes []string `json:"stationsTypes"`
}

func (c *ConstantsClient) FetchConstants() (map[string]string, error) {
	resp, err := http.Get("https://brn-ybus-pubapi.sa.cz/restapi/consts/locations")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("Failed to fetch data, status code: %d", resp.StatusCode))
	}

	var countries []Country
	if err := json.NewDecoder(resp.Body).Decode(&countries); err != nil {
		return nil, err
	}

	stations := make(map[string]string)
	for _, country := range countries {
		for _, city := range country.Cities {
			for _, station := range city.Stations {
				isTrainStation := false
				for _, stationType := range station.StationTypes {
					if stationType == "TRAIN_STATION" {
						isTrainStation = true
						break
					}
				}
				if isTrainStation {
					strID := strconv.FormatInt(station.ID, 10)
					stations[strID] = station.FullName
				}
			}
		}
	}
	log.Default().Printf("Stations fetched: %d", len(stations))
	return stations, nil
}

func RegisterConstantsHooks(lc fx.Lifecycle, service *ConstantsClient) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go service.FetchConstants()
			return nil
		},
		OnStop: nil,
	})
}
