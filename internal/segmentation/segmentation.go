package segmentation

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/service"
)

type SegmentationService struct {
	trainClient  *client.TrainClient
	trainService *service.TrainService
	constants    map[string]string
}

func NewSegmentationService(trainClient *client.TrainClient, trainService *service.TrainService, constantsClient *constants.ConstantsClient) *SegmentationService {
	constMap, _ := constantsClient.FetchConstants()
	return &SegmentationService{
		trainClient: trainClient,
		constants:   constMap,
	}
}

func (s *SegmentationService) FindAvailableSegments(routeID, stationFromID, stationToID, departureDate string) ([][]map[string]string, error) {
	resp, err := s.trainClient.FetchStops(routeID)
	if err != nil {
		log.Println("Failed to fetch stops:", err)
		return nil, err
	}

	stations := resp.Stations

	var currentStation client.Stop

	for _, station := range stations {
		if strconv.Itoa(station.StationID) == stationFromID {
			currentStation = station
			break
		}
	}

	paths, err := s.findPath(currentStation, stationToID, stations, departureDate)
	log.Default().Printf("Found %d paths", len(paths))
	if err != nil {
		log.Println("Failed to find path:", err)
		return nil, err
	}
	sort.Slice(paths, func(i, j int) bool {
		return len(paths[i]) < len(paths[j])
	})

	var allPaths [][]map[string]string

	for _, path := range paths {
		var onePath []map[string]string

		var totalPrice float64
		for _, segment := range path {
			niceSegment := make(map[string]string)

			fromStationName := s.constants[segment["FromStationID"].(string)]
			toStationName := s.constants[segment["ToStationID"].(string)]

			departureTime, _ := time.Parse(time.RFC3339, segment["DepartureTime"].(string))
			arrivalTime, _ := time.Parse(time.RFC3339, segment["ArrivalTime"].(string))
			niceSegment["from"] = fromStationName
			niceSegment["to"] = toStationName
			niceSegment["price"] = fmt.Sprintf("%.2f", segment["Price"].(float64))
			niceSegment["departureTime"] = departureTime.Format("15:04")
			if len(onePath) == 0 {
				departureTime, _ = time.Parse("15:04:05.000", currentStation.Departure)
				niceSegment["departureTime"] = departureTime.Format("15:04")
			}
			niceSegment["arrivalTime"] = arrivalTime.Format("15:04")
			niceSegment["freeSeats"] = fmt.Sprintf("%d", segment["FreeSeats"].(int))
			niceSegment["departureDate"] = segment["DepartureDate"].(string)

			totalPrice += segment["Price"].(float64)
			onePath = append(onePath, niceSegment)
		}
		onePath = append(onePath, map[string]string{"totalPrice": fmt.Sprintf("%.2f", totalPrice)})

		allPaths = append(allPaths, onePath)
	}

	return allPaths, nil
}

func (s *SegmentationService) findPath(currentStation client.Stop, targetStationID string, stations []client.Stop, departureDate string) ([][]map[string]interface{}, error) {

	paths := make([][]map[string]interface{}, 0)
	currPath := make([]map[string]interface{}, 0)
	visited := make(map[string]bool)

	s.findPathRecursive(currentStation, targetStationID, stations, currPath, &paths, visited, departureDate, 0)
	return paths, nil
}

func (s *SegmentationService) findPathRecursive(currentStation client.Stop, targetStationID string, stations []client.Stop, currPath []map[string]interface{}, paths *[][]map[string]interface{}, visited map[string]bool, departureDate string, index int) {
	if strconv.Itoa(currentStation.StationID) == targetStationID {
		newPath := append([]map[string]interface{}{}, currPath...)
		*paths = append(*paths, newPath)
		return
	}

	visited[strconv.Itoa(currentStation.StationID)] = true

	var currStation client.Stop = currentStation
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

func (s *SegmentationService) checkSegment(currentStation, nextStation client.Stop, departureDate string) (map[string]interface{}, error) {

	routes, err := s.trainClient.FetchRoutes(strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID), departureDate, "CZK")
	if err != nil {
		log.Println("Failed to fetch routes:", err)
		return nil, err
	}

	for _, route := range routes {
		parsedCurrentDeparture, _ := time.Parse("15:04:05.000", currentStation.Departure)
		if route.DepartureTime != parsedCurrentDeparture.Format("15:04") {
			log.Printf("Departure time mismatch: %s != %s", route.DepartureTime, parsedCurrentDeparture.Format("15:04"))
			continue
		}
		rID, _ := strconv.Atoi(route.ID)
		details, err := s.trainService.GetRouteDetails(rID, strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID))
		if err != nil {
			log.Println("Failed to fetch free seats:", err)
			continue
		}

		if details.FreeSeatsCount > 0 {
			segment := map[string]interface{}{
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
