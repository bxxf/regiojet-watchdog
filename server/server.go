package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/service"
	"github.com/google/uuid"
	"go.uber.org/fx"
)

type Server struct {
	trainClient  *client.TrainClient
	trainService *service.TrainService
	config       config.Config
	database     *database.DatabaseClient
	constants    map[string]string
}

func NewServer(trainClient *client.TrainClient, trainService *service.TrainService, config config.Config, database *database.DatabaseClient, constantsClient *constants.ConstantsClient) *Server {
	constMap, _ := constantsClient.FetchConstants()
	return &Server{
		trainClient:  trainClient,
		trainService: trainService,
		config:       config,
		database:     database,
		constants:    constMap,
	}
}

func (s *Server) run() {
	http.HandleFunc("/routes", s.getRoutesHandler)
	http.HandleFunc(("/watchdog"), s.watchdogHandler)
	http.HandleFunc("/constants", s.constantsHandler)
	http.HandleFunc("/allSegments", s.findAllSegmentsHandler)

	port := ":7900"
	log.Printf("Server is running on port %s...\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func (s *Server) getRoutesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stationFromID := r.URL.Query().Get("stationFromID")
	stationToID := r.URL.Query().Get("stationToID")
	departureDateInput := r.URL.Query().Get("departureDate")

	routes, err := s.trainClient.FetchRoutes(stationFromID, stationToID, departureDateInput, "CZK")
	if err != nil {
		http.Error(w, "Failed to fetch routes", http.StatusInternalServerError)
		log.Println("Failed to fetch routes:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(routes); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}

func (s *Server) watchdogHandler(w http.ResponseWriter, r *http.Request) {
	body := struct {
		StationFromID string `json:"stationFromID"`
		StationToID   string `json:"stationToID"`
		RouteID       string `json:"routeID"`
		WebhookURL    string `json:"webhookURL"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		log.Println("Failed to parse request body:", err)
		return
	}

	routeInt, _ := strconv.Atoi(body.RouteID)
	routeDetails, err := s.trainService.GetRouteDetails(routeInt, body.StationFromID, body.StationToID)
	if err != nil {
		http.Error(w, "Failed to fetch route details", http.StatusInternalServerError)
		log.Println("Failed to fetch route details:", err)
		return
	}
	departureTime, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	departureDuration := departureTime.Sub(time.Now())

	uuid := "watchdog:" + uuid.New().String()
	s.database.RedisClient.Set(context.Background(), uuid, body.WebhookURL+";;"+body.StationFromID+";;"+body.StationToID+";;"+body.RouteID, departureDuration)

	res := struct {
		Message string `json:"message"`
	}{
		Message: "Watchdog set successfully.",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}

}

func (s *Server) periodicallyCheck() {
	// Define the interval between each check, e.g., 5 minutes
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			keys, err := s.database.RedisClient.Keys(context.Background(), "watchdog:*").Result()
			if err != nil {
				log.Println("Failed to fetch keys:", err)
				continue
			}

			for _, key := range keys {
				value, err := s.database.RedisClient.Get(context.Background(), key).Result()
				if err != nil {
					log.Println("Failed to fetch value for key", key, ":", err)
					continue
				}

				parts := strings.Split(value, ";;")
				if len(parts) != 4 {
					log.Println("Invalid value format for key", key)
					continue
				}
				webhookURL, stationFromID, stationToID, routeIDStr := parts[0], parts[1], parts[2], parts[3]

				routeID, err := strconv.Atoi(routeIDStr)
				if err != nil {
					log.Println("Failed to parse routeID:", err)
					continue
				}

				freeSeatsResponse, err := s.trainClient.GetFreeSeats(routeID, stationFromID, stationToID)
				routeDetails, err := s.trainService.GetRouteDetails(routeID, stationFromID, stationToID)
				if err != nil {
					log.Println("Failed to fetch route details:", err)
					continue
				}

				if routeDetails.FreeSeatsCount > 0 {
					s.trainService.NotifyDiscord(freeSeatsResponse, *routeDetails, webhookURL)
				} else {
					departureTime, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
					departureDate := departureTime.Format("02.01.2006")
					availableSegments, err := s.FindAvailableSegments(strconv.Itoa(routeID), stationFromID, stationToID, departureDate)
					if err != nil {
						log.Println("Failed to fetch available segments:", err)
						continue
					}
					if len(availableSegments) > 0 {
						s.trainService.NotifyDiscordAlternatives(availableSegments, webhookURL)
					}
				}
			}
		}
	}
}

func (s *Server) constantsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.constants); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}

func (s *Server) findAllSegmentsHandler(w http.ResponseWriter, r *http.Request) {
	stationFromID := r.URL.Query().Get("stationFromID")
	stationToID := r.URL.Query().Get("stationToID")
	routeID := r.URL.Query().Get("routeID")
	departureDate := r.URL.Query().Get("departureDate")

	availableSegments, err := s.FindAvailableSegments(routeID, stationFromID, stationToID, departureDate)
	if err != nil {
		http.Error(w, "Failed to fetch available segments", http.StatusInternalServerError)
		log.Println("Failed to fetch available segments:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(availableSegments); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
func (s *Server) FindAvailableSegments(routeID, stationFromID, stationToID, departureDate string) ([][]map[string]string, error) {
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

			totalPrice += segment["Price"].(float64)
			onePath = append(onePath, niceSegment)
		}
		onePath = append(onePath, map[string]string{"totalPrice": fmt.Sprintf("%.2f", totalPrice)})

		allPaths = append(allPaths, onePath)
	}

	return allPaths, nil
}

func (s *Server) findPath(currentStation client.Stop, targetStationID string, stations []client.Stop, departureDate string) ([][]map[string]interface{}, error) {

	paths := make([][]map[string]interface{}, 0)
	currPath := make([]map[string]interface{}, 0)
	visited := make(map[string]bool)

	s.findPathRecursive(currentStation, targetStationID, stations, currPath, &paths, visited, departureDate, 0)
	return paths, nil
}

func (s *Server) findPathRecursive(currentStation client.Stop, targetStationID string, stations []client.Stop, currPath []map[string]interface{}, paths *[][]map[string]interface{}, visited map[string]bool, departureDate string, index int) {
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

func (s *Server) checkSegment(currentStation, nextStation client.Stop, departureDate string) (map[string]interface{}, error) {

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
				"Price":         details.PriceFrom,
			}
			return segment, nil
		}
	}

	return nil, fmt.Errorf("No free seats available from station %s to station %s", strconv.Itoa(currentStation.StationID), strconv.Itoa(nextStation.StationID))
}

func RegisterServerHooks(lc fx.Lifecycle, server *Server) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go server.run()
			go server.periodicallyCheck()
			return nil
		},
		OnStop: nil,
	})
}
