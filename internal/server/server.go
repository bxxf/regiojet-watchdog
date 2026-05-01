package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/bxxf/regiojet-watchdog/internal/models"
	"github.com/bxxf/regiojet-watchdog/internal/ui"
	"go.uber.org/fx"
)

type Server struct {
	trainClient *client.TrainClient
	config      config.Config
	constants   map[string]string
	stations    []ui.Station
	database    *database.DatabaseClient
}

type watchdogRequest struct {
	StationFromID string
	StationToID   string
	RouteID       string
	WebhookURL    string
	WebhookType   string
	CheckSegments bool
}

type requestError struct {
	status  int
	message string
}

func (e requestError) Error() string {
	return e.message
}

func NewServer(trainClient *client.TrainClient, config config.Config, constantsClient *constants.ConstantsClient, database *database.DatabaseClient) *Server {
	constMap, _ := constantsClient.FetchConstants()
	return &Server{
		trainClient: trainClient,
		config:      config,
		constants:   constMap,
		stations:    stationsFromConstants(constMap),
		database:    database,
	}
}

func stationsFromConstants(constants map[string]string) []ui.Station {
	stations := make([]ui.Station, 0, len(constants))
	for id, name := range constants {
		stations = append(stations, ui.Station{
			ID:   id,
			Name: name,
		})
	}
	sort.Slice(stations, func(i, j int) bool {
		return stations[i].Name < stations[j].Name
	})
	return stations
}

func (s *Server) pageData() ui.PageData {
	watchdogs, err := s.watchdogsForUI(context.Background())
	if err != nil {
		log.Println("Failed to load watchdogs:", err)
	}
	return ui.PageData{
		Stations:      s.stations,
		Watchdogs:     watchdogs,
		Currency:      "CZK",
		WebhookType:   "discord",
		CheckSegments: false,
	}
}

func (s *Server) run() {
	assetsHandler := http.StripPrefix("/assets/", ui.AssetsHandler())
	http.Handle("/assets/", assetsHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" {
			if err := ui.Page(s.pageData()).Render(r.Context(), w); err != nil {
				http.Error(w, "Failed to render page", http.StatusInternalServerError)
				log.Println("Failed to render page:", err)
			}
			return
		}
		http.NotFound(w, r)
	})

	http.HandleFunc("/ui/stations", s.stationOptionsHandler)
	http.HandleFunc("/ui/station-picker", s.stationPickerHandler)
	http.HandleFunc("/ui/search-panel/swap", s.swapSearchPanelHandler)
	http.HandleFunc("/ui/webhook-help", s.webhookHelpHandler)
	http.HandleFunc("/ui/routes", s.uiRoutesHandler)
	http.HandleFunc("/ui/watchdog", s.uiWatchdogSetHandler)
	http.HandleFunc("/ui/watchdog/remove", s.uiWatchdogRemoveHandler)
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
	currency := normalizeCurrency(r.URL.Query().Get("currency"))

	routes, err := s.trainClient.FetchRoutes(stationFromID, stationToID, departureDateInput, currency)
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

func (s *Server) stationOptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := r.URL.Query().Get("target")
	if target != "from" && target != "to" {
		http.Error(w, "Invalid station target", http.StatusBadRequest)
		return
	}

	query := r.URL.Query().Get(target + "Search")
	selectedID := r.URL.Query().Get(target + "StationID")
	selectedName := ui.StationLabel(s.stations, selectedID)
	stations := ui.FilterStations(s.stations, query)
	normalizedQuery := strings.TrimSpace(query)
	clearSelection := selectedID != "" && normalizedQuery != selectedName
	if selectedName != "" && strings.EqualFold(normalizedQuery, selectedName) {
		stations = s.stations
	}

	component := ui.StationOptions(ui.StationOptionsData{
		Target:         target,
		Query:          query,
		Stations:       stations,
		ClearSelection: clearSelection,
	})
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render station options", http.StatusInternalServerError)
		log.Println("Failed to render station options:", err)
	}
}

func (s *Server) stationPickerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := r.URL.Query().Get("target")
	if target != "from" && target != "to" {
		http.Error(w, "Invalid station target", http.StatusBadRequest)
		return
	}

	stationID := r.URL.Query().Get("stationID")
	label := "From"
	if target == "to" {
		label = "To"
	}

	if err := ui.StationPicker(target, label, stationID, ui.StationLabel(s.stations, stationID)).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render station picker", http.StatusInternalServerError)
		log.Println("Failed to render station picker:", err)
	}
}

func (s *Server) swapSearchPanelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	fromID := r.FormValue("fromStationID")
	toID := r.FormValue("toStationID")
	data := s.pageData()
	data.FromID = toID
	data.FromName = ui.StationLabel(s.stations, toID)
	data.ToID = fromID
	data.ToName = ui.StationLabel(s.stations, fromID)
	data.Departure = r.FormValue("departure")
	data.Currency = normalizeCurrency(r.FormValue("currency"))

	if err := ui.SearchPanel(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render search panel", http.StatusInternalServerError)
		log.Println("Failed to render search panel:", err)
	}
}

func (s *Server) webhookHelpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	webhookType := r.URL.Query().Get("webhookType")
	if webhookType != "discord" && webhookType != "simple" {
		webhookType = "discord"
	}

	if err := ui.WebhookHelp(webhookType, r.URL.Query().Get("checkSegments") == "1").Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render webhook help", http.StatusInternalServerError)
		log.Println("Failed to render webhook help:", err)
	}
}

func (s *Server) uiRoutesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.routeListDataFromRequest(r)
	if data.FromID == "" || data.ToID == "" || data.Departure == "" {
		data.Error = "Choose both stations and a departure date."
	} else if data.FromID == data.ToID {
		data.Error = "Choose two different stations."
	} else {
		routes, err := s.trainClient.FetchRoutes(data.FromID, data.ToID, departureDateForAPI(data.Departure), data.Currency)
		if err != nil {
			data.Error = "Failed to fetch routes."
			log.Println("Failed to fetch UI routes:", err)
		} else {
			data.Routes = routes
		}
	}

	s.renderRouteListWithWatchdogs(w, r, data)
}

func (s *Server) uiWatchdogSetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	data := s.routeListDataFromRequest(r)
	req := watchdogRequest{
		StationFromID: data.FromID,
		StationToID:   data.ToID,
		RouteID:       r.FormValue("routeID"),
		WebhookURL:    strings.TrimSpace(r.FormValue("webhookURL")),
		WebhookType:   strings.TrimSpace(r.FormValue("webhookType")),
		CheckSegments: r.FormValue("checkSegments") == "1",
	}
	if err := s.setWatchdog(r.Context(), req); err != nil {
		data.Error = err.Error()
	} else {
		routes, err := s.trainClient.FetchRoutes(data.FromID, data.ToID, departureDateForAPI(data.Departure), data.Currency)
		if err != nil {
			data.Error = "Watchdog was created, but routes could not be refreshed."
			log.Println("Failed to refresh routes after watchdog set:", err)
		} else {
			data.Routes = routes
			if s.renderRouteRowWithWatchdogs(w, r, data, req.RouteID) {
				return
			}
		}
	}

	s.renderRouteListWithWatchdogs(w, r, data)
}

func (s *Server) uiWatchdogRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	data := s.routeListDataFromRequest(r)
	if err := s.removeWatchdog(r.Context(), r.FormValue("routeID")); err != nil {
		data.Error = err.Error()
	} else if data.FromID != "" && data.ToID != "" && data.Departure != "" {
		routes, err := s.trainClient.FetchRoutes(data.FromID, data.ToID, departureDateForAPI(data.Departure), data.Currency)
		if err != nil {
			data.Error = "Watchdog was removed, but routes could not be refreshed."
			log.Println("Failed to refresh routes after watchdog removal:", err)
		} else {
			data.Routes = routes
			if s.renderRouteRowWithWatchdogs(w, r, data, r.FormValue("routeID")) {
				return
			}
		}
	}

	if r.Header.Get("HX-Target") == "watchdogs-panel" {
		s.renderWatchdogPanel(w, r)
		return
	}
	s.renderRouteListWithWatchdogs(w, r, data)
}

func (s *Server) renderRouteRowWithWatchdogs(w http.ResponseWriter, r *http.Request, data ui.RouteListData, routeID string) bool {
	for _, route := range data.Routes {
		if route.ID != routeID {
			continue
		}

		w.Header().Set("HX-Retarget", "#route-"+routeID)
		if err := ui.RouteRow(route, data).Render(r.Context(), w); err != nil {
			http.Error(w, "Failed to render route", http.StatusInternalServerError)
			log.Println("Failed to render route row:", err)
			return true
		}

		watchdogs, err := s.watchdogsForUI(r.Context())
		if err != nil {
			log.Println("Failed to refresh watchdogs:", err)
			return true
		}
		if err := ui.WatchdogPanelOOB(watchdogs).Render(r.Context(), w); err != nil {
			log.Println("Failed to render watchdog panel:", err)
		}
		return true
	}
	return false
}

func (s *Server) renderRouteListWithWatchdogs(w http.ResponseWriter, r *http.Request, data ui.RouteListData) {
	if err := ui.RouteList(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render routes", http.StatusInternalServerError)
		log.Println("Failed to render routes:", err)
		return
	}
	watchdogs, err := s.watchdogsForUI(r.Context())
	if err != nil {
		log.Println("Failed to refresh watchdogs:", err)
		return
	}
	if err := ui.WatchdogPanelOOB(watchdogs).Render(r.Context(), w); err != nil {
		log.Println("Failed to render watchdog panel:", err)
	}
}

func (s *Server) renderWatchdogPanel(w http.ResponseWriter, r *http.Request) {
	watchdogs, err := s.watchdogsForUI(r.Context())
	if err != nil {
		http.Error(w, "Failed to render watchdogs", http.StatusInternalServerError)
		log.Println("Failed to load watchdogs:", err)
		return
	}
	if err := ui.WatchdogPanel(watchdogs).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render watchdogs", http.StatusInternalServerError)
		log.Println("Failed to render watchdog panel:", err)
	}
}

func (s *Server) routeListDataFromRequest(r *http.Request) ui.RouteListData {
	fromID := r.FormValue("fromStationID")
	toID := r.FormValue("toStationID")
	departure := r.FormValue("departure")
	currency := normalizeCurrency(r.FormValue("currency"))
	return ui.RouteListData{
		FromID:    fromID,
		FromName:  ui.StationLabel(s.stations, fromID),
		ToID:      toID,
		ToName:    ui.StationLabel(s.stations, toID),
		Departure: departure,
		Currency:  currency,
	}
}

func normalizeCurrency(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "EUR":
		return "EUR"
	case "PLN":
		return "PLN"
	case "HUF":
		return "HUF"
	default:
		return "CZK"
	}
}

func departureDateForAPI(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02.01.2006")
}

func (s *Server) watchdogSetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	err := s.setWatchdog(r.Context(), watchdogRequest{
		StationFromID: body.StationFromID,
		StationToID:   body.StationToID,
		RouteID:       body.RouteID,
		WebhookURL:    body.WebhookURL,
		WebhookType:   body.WebhookType,
		CheckSegments: body.CheckSegments,
	})
	if err != nil {
		http.Error(w, err.Error(), watchdogErrorStatus(err))
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

func (s *Server) setWatchdog(ctx context.Context, req watchdogRequest) error {
	req.StationFromID = strings.TrimSpace(req.StationFromID)
	req.StationToID = strings.TrimSpace(req.StationToID)
	req.RouteID = strings.TrimSpace(req.RouteID)
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)
	req.WebhookType = strings.TrimSpace(req.WebhookType)

	if req.StationFromID == "" || req.StationToID == "" {
		return requestError{status: http.StatusBadRequest, message: "Choose both stations before creating a watchdog."}
	}
	if req.RouteID == "" {
		return requestError{status: http.StatusBadRequest, message: "Choose a route before creating a watchdog."}
	}
	if req.WebhookURL == "" {
		return requestError{status: http.StatusBadRequest, message: "Webhook URL is missing."}
	}
	if req.WebhookType != "discord" && req.WebhookType != "simple" {
		return requestError{status: http.StatusBadRequest, message: "Choose Discord or HTTP POST as the webhook type."}
	}

	routeInt, err := strconv.Atoi(req.RouteID)
	if err != nil {
		return requestError{status: http.StatusBadRequest, message: "routeID must be a number"}
	}

	routeDetails, err := s.trainClient.GetRouteDetails(routeInt, req.StationFromID, req.StationToID)
	if err != nil {
		log.Println("Failed to fetch route details:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to fetch route details"}
	}

	departureTime, err := time.Parse(time.RFC3339, routeDetails.DepartureTime)
	if err != nil {
		log.Println("Failed to parse departure time:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to parse departure time"}
	}

	departureDuration := time.Until(departureTime)
	if departureDuration <= 0 {
		return requestError{status: http.StatusBadRequest, message: "departure has already passed"}
	}

	jsonWebhook, err := json.Marshal(models.Webhook{
		WebhookURL:    req.WebhookURL,
		WebhookType:   req.WebhookType,
		StationFromID: req.StationFromID,
		StationToID:   req.StationToID,
		RouteID:       req.RouteID,
		CheckSegments: req.CheckSegments,
	})
	if err != nil {
		log.Println("Failed to marshal JSON payload:", err)
		return requestError{status: http.StatusBadRequest, message: "failed to marshal JSON payload"}
	}

	subscriptionID, err := randomID()
	if err != nil {
		log.Println("Failed to generate watchdog id:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to generate watchdog id"}
	}

	key := "watchdog:" + req.RouteID + ":" + subscriptionID
	if err := s.database.RedisClient.Set(ctx, key, jsonWebhook, departureDuration).Err(); err != nil {
		log.Println("Failed to store watchdog:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to store watchdog"}
	}
	return nil
}

func watchdogErrorStatus(err error) int {
	var reqErr requestError
	if errors.As(err, &reqErr) {
		return reqErr.status
	}
	return http.StatusInternalServerError
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

	if err := s.removeWatchdog(r.Context(), body.RouteID); err != nil {
		http.Error(w, err.Error(), watchdogErrorStatus(err))
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

func (s *Server) removeWatchdog(ctx context.Context, routeID string) error {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return requestError{status: http.StatusBadRequest, message: "routeID is required"}
	}

	keys, err := s.watchdogKeysForRoute(ctx, routeID)
	if err != nil {
		log.Println("Failed to find watchdog:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to remove watchdog"}
	}
	if len(keys) == 0 {
		return requestError{status: http.StatusNotFound, message: "watchdog not found"}
	}

	deleted, err := s.database.RedisClient.Del(ctx, keys...).Result()
	if err != nil {
		log.Println("Failed to remove watchdog:", err)
		return requestError{status: http.StatusInternalServerError, message: "failed to remove watchdog"}
	}
	if deleted == 0 {
		return requestError{status: http.StatusNotFound, message: "watchdog not found"}
	}
	return nil
}

func (s *Server) watchdogKeysForRoute(ctx context.Context, routeID string) ([]string, error) {
	var keys []string
	iter := s.database.RedisClient.Scan(ctx, 0, "watchdog:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if key == "watchdog:"+routeID || strings.HasPrefix(key, "watchdog:"+routeID+":") {
			keys = append(keys, key)
			continue
		}

		value, err := s.database.RedisClient.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if watchdogValueRouteID(value) == routeID {
			keys = append(keys, key)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func watchdogValueRouteID(value string) string {
	var webhook models.Webhook
	if err := json.Unmarshal([]byte(value), &webhook); err == nil {
		return webhook.RouteID
	}

	parts := strings.Split(value, ";;")
	if len(parts) == 4 {
		return parts[3]
	}
	return ""
}

func (s *Server) watchdogsForUI(ctx context.Context) ([]ui.WatchdogView, error) {
	var watchdogs []ui.WatchdogView
	iter := s.database.RedisClient.Scan(ctx, 0, "watchdog:*", 0).Iterator()
	for iter.Next(ctx) {
		value, err := s.database.RedisClient.Get(ctx, iter.Val()).Result()
		if err != nil {
			return nil, err
		}
		webhook, err := parseStoredWebhook(value)
		if err != nil {
			log.Println("Failed to parse watchdog value:", err)
			continue
		}

		view := ui.WatchdogView{
			RouteID:       webhook.RouteID,
			FromName:      ui.StationLabel(s.stations, webhook.StationFromID),
			ToName:        ui.StationLabel(s.stations, webhook.StationToID),
			WebhookType:   webhook.WebhookType,
			CheckSegments: webhook.CheckSegments,
		}
		if view.FromName == "" {
			view.FromName = "Unknown station"
		}
		if view.ToName == "" {
			view.ToName = "Unknown station"
		}

		routeID, err := strconv.Atoi(webhook.RouteID)
		if err == nil {
			if details, err := s.trainClient.GetRouteDetails(routeID, webhook.StationFromID, webhook.StationToID); err == nil {
				view.DepartureTime = formatWatchdogTime(details.DepartureTime)
				view.ArrivalTime = formatWatchdogTime(details.ArrivalTime)
			}
		}

		watchdogs = append(watchdogs, view)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(watchdogs, func(i, j int) bool {
		if watchdogs[i].DepartureTime == "" || watchdogs[j].DepartureTime == "" {
			return watchdogs[i].FromName < watchdogs[j].FromName
		}
		return watchdogs[i].DepartureTime < watchdogs[j].DepartureTime
	})
	return watchdogs, nil
}

func parseStoredWebhook(value string) (models.Webhook, error) {
	var webhook models.Webhook
	if err := json.Unmarshal([]byte(value), &webhook); err == nil {
		return webhook, nil
	}

	parts := strings.Split(value, ";;")
	if len(parts) != 4 {
		return models.Webhook{}, errors.New("invalid watchdog value")
	}
	return models.Webhook{
		WebhookURL:    parts[0],
		StationFromID: parts[1],
		StationToID:   parts[2],
		RouteID:       parts[3],
		WebhookType:   "discord",
		CheckSegments: true,
	}, nil
}

func formatWatchdogTime(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return parsed.Format("15:04")
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
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
