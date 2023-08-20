package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/google/uuid"
	"go.uber.org/fx"
)

type Server struct {
	trainClient *client.TrainClient
	config      config.Config
	constants   map[string]string
	database    *database.DatabaseClient
}

func NewServer(trainClient *client.TrainClient, config config.Config, constantsClient *constants.ConstantsClient, database *database.DatabaseClient) *Server {
	constMap, _ := constantsClient.FetchConstants()
	return &Server{
		trainClient: trainClient,
		config:      config,
		constants:   constMap,
		database:    database,
	}
}

func (s *Server) run() {
	http.HandleFunc("/routes", s.getRoutesHandler)
	http.HandleFunc(("/watchdog"), s.watchdogHandler)
	http.HandleFunc("/constants", s.constantsHandler)

	port := s.config.Port
	log.Printf("Server is running on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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
	routeDetails, err := s.trainClient.GetRouteDetails(routeInt, body.StationFromID, body.StationToID)
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

func RegisterServerHooks(lc fx.Lifecycle, server *Server) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go server.run()
			return nil
		},
		OnStop: nil,
	})
}
