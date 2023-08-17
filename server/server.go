package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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
	trainClient     *client.TrainClient
	trainService    *service.TrainService
	config          config.Config
	database        *database.DatabaseClient
	constantsClient *constants.ConstantsClient
}

func NewServer(trainClient *client.TrainClient, trainService *service.TrainService, config config.Config, database *database.DatabaseClient, constantsClient *constants.ConstantsClient) *Server {
	return &Server{
		trainClient:     trainClient,
		trainService:    trainService,
		config:          config,
		database:        database,
		constantsClient: constantsClient,
	}
}

func (s *Server) run() {
	http.HandleFunc("/routes", s.getRoutesHandler)
	http.HandleFunc(("/watchdog"), s.watchdogHandler)
	http.HandleFunc("/constants", s.constantsHandler)

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

	uuid := "watchdog:" + uuid.New().String()
	routeInt, _ := strconv.Atoi(body.RouteID)
	routeDetails, err := s.trainService.GetRouteDetails(routeInt, body.StationFromID, body.StationToID)
	if err != nil {
		http.Error(w, "Failed to fetch route details", http.StatusInternalServerError)
		log.Println("Failed to fetch route details:", err)
		return
	}
	departureTime, _ := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	departureDuration := departureTime.Sub(time.Now())

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

				// Parse the data
				parts := strings.Split(value, ";;")
				if len(parts) != 4 {
					log.Println("Invalid value format for key", key)
					continue
				}
				webhookURL, stationFromID, stationToID, routeIDStr := parts[0], parts[1], parts[2], parts[3]

				// Parse the route ID to int
				routeID, err := strconv.Atoi(routeIDStr)
				if err != nil {
					log.Println("Failed to parse routeID:", err)
					continue
				}

				freeSeatsResponse, err := s.trainClient.GetFreeSeats(routeID, stationFromID, stationToID)
				// Call the GetRouteDetails method with the parsed data
				routeDetails, err := s.trainService.GetRouteDetails(routeID, stationFromID, stationToID)
				if err != nil {
					log.Println("Failed to fetch route details:", err)
					continue
				}

				s.trainService.NotifyDiscord(freeSeatsResponse, *routeDetails, webhookURL)
			}
		}
	}
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

func (s *Server) constantsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	constants, err := s.constantsClient.FetchConstants()
	if err != nil {
		http.Error(w, "Failed to fetch constants", http.StatusInternalServerError)
		log.Println("Failed to fetch constants:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(constants); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
