package segmentation

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/models"
)

const timeFormat = "15:04:05.000"

type SegmentationService struct {
	trainClient *client.TrainClient
	constants   map[string]string
}

func NewSegmentationService(trainClient *client.TrainClient, constantsClient *constants.ConstantsClient) (*SegmentationService, error) {
	constMap, err := constantsClient.FetchConstants()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch constants: %v", err)
	}

	return &SegmentationService{
		trainClient: trainClient,
		constants:   constMap,
	}, nil
}

func (s *SegmentationService) FindAvailableSegments(routeID, stationFromID, stationToID, departureDate string) ([][]map[string]string, error) {
	stationsResp, err := s.trainClient.FetchStops(routeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stops: %v", err)
	}

	var currentStation models.Stop
	for _, station := range stationsResp.Stations {
		if strconv.Itoa(station.StationID) == stationFromID {
			currentStation = station
			break
		}
	}

	paths, err := s.findPath(currentStation, stationToID, stationsResp.Stations, departureDate)
	if err != nil {
		return nil, fmt.Errorf("failed to find path: %v", err)
	}

	return s.formatPaths(paths), nil
}

func (s *SegmentationService) formatPaths(paths [][]map[string]interface{}) [][]map[string]string {
	var allPaths [][]map[string]string

	for _, path := range paths {
		var onePath []map[string]string
		var totalPrice float64

		for _, segment := range path {
			niceSegment := make(map[string]string)
			fromStationName, ok := s.constants[segment["FromStationID"].(string)]
			if !ok {
				log.Printf("Station ID not found in constants: %v", segment["FromStationID"])
				continue
			}

			toStationName, ok := s.constants[segment["ToStationID"].(string)]
			if !ok {
				log.Printf("Station ID not found in constants: %v", segment["ToStationID"])
				continue
			}

			departureTime, err := time.Parse(time.RFC3339, segment["DepartureTime"].(string))
			if err != nil {
				log.Printf("Failed to parse departure time: %v", err)
				continue
			}

			arrivalTime, err := time.Parse(time.RFC3339, segment["ArrivalTime"].(string))
			if err != nil {
				log.Printf("Failed to parse arrival time: %v", err)
				continue
			}

			niceSegment["from"] = fromStationName
			niceSegment["to"] = toStationName
			niceSegment["price"] = fmt.Sprintf("%.2f", segment["Price"].(float64))
			niceSegment["departureTime"] = departureTime.Format("15:04")
			niceSegment["arrivalTime"] = arrivalTime.Format("15:04")
			niceSegment["freeSeats"] = fmt.Sprintf("%d", segment["FreeSeats"].(int))
			niceSegment["departureDate"] = segment["DepartureDate"].(string)

			totalPrice += segment["Price"].(float64)
			onePath = append(onePath, niceSegment)
		}
		onePath = append(onePath, map[string]string{"totalPrice": fmt.Sprintf("%.2f", totalPrice)})
		allPaths = append(allPaths, onePath)
	}

	return allPaths
}

func (s *SegmentationService) findPath(currentStation models.Stop, targetStationID string, stations []models.Stop, departureDate string) ([][]map[string]interface{}, error) {

	paths := make([][]map[string]interface{}, 0)
	currPath := make([]map[string]interface{}, 0)
	visited := make(map[string]bool)

	s.findPathRecursive(currentStation, targetStationID, stations, currPath, &paths, visited, departureDate, 0)
	return paths, nil
}

func (s *SegmentationService) findPathRecursive(currentStation models.Stop, targetStationID string, stations []models.Stop, currPath []map[string]interface{}, paths *[][]map[string]interface{}, visited map[string]bool, departureDate string, index int) {
	if strconv.Itoa(currentStation.StationID) == targetStationID {
		newPath := append([]map[string]interface{}{}, currPath...)
		*paths = append(*paths, newPath)
		return
	}

	visited[strconv.Itoa(currentStation.StationID)] = true

	var currStation models.Stop = currentStation
	for _, station := range stations {
		if station.Index < currStation.Index {
			continue
		}
		if strconv.Itoa(station.StationID) == strconv.Itoa(currentStation.StationID) {
			currStation = station
			break
		}
	}

	for _, nextStation := range stations {

		if visited[strconv.Itoa(nextStation.StationID)] {
			continue
		}

		if nextStation.Index > currStation.Index {
			segment, err := s.checkSegment(currentStation, nextStation, departureDate)
			if err == nil && segment["FreeSeats"].(int) > 0 {
				currPath = append(currPath, segment)
				s.findPathRecursive(nextStation, targetStationID, stations, currPath, paths, visited, departureDate, index+1)
				currPath = currPath[:len(currPath)-1] // Remove the last segment to backtrack
			}
		}
	}
	visited[strconv.Itoa(currentStation.StationID)] = false
}

func (s *SegmentationService) checkSegment(currentStation, nextStation models.Stop, departureDate string) (map[string]interface{}, error) {

	routes, err := s.trainClient.FetchRoutes(strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID), departureDate, "CZK")
	if err != nil {
		log.Println("Failed to fetch routes:", err)
		return nil, err
	}

	for _, route := range routes {
		parsedCurrentDeparture, _ := time.Parse("15:04:05.000", currentStation.Departure)
		if route.DepartureTime != parsedCurrentDeparture.Format("15:04") {
			log.Printf("comparing routes: %s != %s", route.DepartureTime, parsedCurrentDeparture.Format("15:04"))
			continue
		}
		rID, _ := strconv.Atoi(route.ID)
		details, err := s.trainClient.GetRouteDetails(rID, strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID))
		if err != nil {
			log.Println("Failed to fetch free seats:", err)
			continue
		}

		var segment map[string]interface{}

		if details.FreeSeatsCount > 0 {
			segment = map[string]interface{}{
				"FromStationID": strconv.Itoa(currentStation.StationID),
				"ToStationID":   strconv.Itoa(nextStation.StationID),
				"RouteID":       route.ID,
				"FreeSeats":     details.FreeSeatsCount,
				"DepartureTime": details.DepartureTime,
				"ArrivalTime":   details.ArrivalTime,
				"DepartureDate": departureDate,
				"Price":         details.PriceFrom,
			}
			return segment, nil
		}

	}

	return nil, fmt.Errorf("No free seats available from station %s to station %s", strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID))
}
