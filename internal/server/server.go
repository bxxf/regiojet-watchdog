package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/bxxf/regiojet-watchdog/internal/client"
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/bxxf/regiojet-watchdog/internal/constants"
	"github.com/bxxf/regiojet-watchdog/internal/database"
	"github.com/bxxf/regiojet-watchdog/internal/models"
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
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var files = map[string]string{
			"/":           "./tpl/index.html",
			"/index.html": "./tpl/index.html",
			"/alpine.js":  "./tpl/alpine.js",
			"/style.css":  "./tpl/style.css",
		}
		path := r.URL.Path
		file, ok := files[path]
		if !ok {
			http.NotFound(w, r)
		} else {
			http.ServeFile(w, r, file)
		}
	})

	http.HandleFunc("/routes", s.getRoutesHandler)
	http.HandleFunc("/watchdog", s.watchdogSetHandler)
	http.HandleFunc("/watchdog/remove", s.watchdogRemoveHandler)
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

func (s *Server) watchdogSetHandler(w http.ResponseWriter, r *http.Request) {
	body := struct {
		StationFromID string `json:"stationFromID"`
		StationToID   string `json:"stationToID"`
		RouteID       string `json:"routeID"`
		WebhookURL    string `json:"webhookURL"`
		WebhookType   string `json:"webhookType"`
		CheckSegments bool   `json:"checkSegments"`
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

	departureTime, err := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	if err != nil {
		http.Error(w, "Failed to parse departure time", http.StatusInternalServerError)
		log.Println("Failed to parse departure time:", err)
		return
	}

	if body.WebhookType != "discord" && body.WebhookType != "simple" {
		http.Error(w, "WebhookType accepts only discord or simple as value", http.StatusBadRequest)
		return
	}

	departureDuration := time.Until(departureTime)
	if departureDuration <= 0 {
		http.Error(w, "Departure has already passed", http.StatusBadRequest)
		return
	}

	jsonWebhook, err := json.Marshal(models.Webhook{
		WebhookURL:    body.WebhookURL,
		WebhookType:   body.WebhookType,
		StationFromID: body.StationFromID,
		StationToID:   body.StationToID,
		RouteID:       body.RouteID,
		CheckSegments: body.CheckSegments,
	})
	if err != nil {
		http.Error(w, "Failed to marshal JSON payload", http.StatusBadRequest)
		log.Println("Failed to marshal JSON payload", err)
		return
	}

	key := "watchdog:" + fmt.Sprint(routeInt)
	if err := s.database.RedisClient.Set(context.Background(), key, jsonWebhook, departureDuration).Err(); err != nil {
		http.Error(w, "Failed to store watchdog", http.StatusInternalServerError)
		log.Println("Failed to store watchdog:", err)
		return
	}

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

func (s *Server) watchdogRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := struct {
		RouteID string `json:"routeID"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		log.Println("Failed to parse request body:", err)
		return
	}

	if body.RouteID == "" {
		http.Error(w, "routeID is required", http.StatusBadRequest)
		return
	}

	key := "watchdog:" + body.RouteID
	deleted, err := s.database.RedisClient.Del(context.Background(), key).Result()
	if err != nil {
		http.Error(w, "Failed to remove watchdog", http.StatusInternalServerError)
		log.Println("Failed to remove watchdog:", err)
		return
	}
	if deleted == 0 {
		http.Error(w, "Watchdog not found", http.StatusNotFound)
		return
	}

	res := struct {
		Message string `json:"message"`
	}{
		Message: "Watchdog removed successfully.",
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
