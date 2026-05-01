package ui

import (
	"fmt"
	"strings"

	"github.com/bxxf/regiojet-watchdog/internal/models"
)

type Station struct {
	ID   string
	Name string
}

type PageData struct {
	Stations      []Station
	Watchdogs     []WatchdogView
	FromID        string
	FromName      string
	ToID          string
	ToName        string
	Departure     string
	Currency      string
	WebhookURL    string
	WebhookType   string
	CheckSegments bool
}

type WatchdogView struct {
	RouteID       string
	FromName      string
	ToName        string
	DepartureTime string
	ArrivalTime   string
	WebhookType   string
	CheckSegments bool
}

type StationOptionsData struct {
	Target         string
	Stations       []Station
	Query          string
	ClearSelection bool
}

type RouteListData struct {
	Routes     []models.Route
	FromID     string
	FromName   string
	ToID       string
	ToName     string
	Departure  string
	Currency   string
	Error      string
	Processing bool
}

func StationLabel(stations []Station, id string) string {
	for _, station := range stations {
		if station.ID == id {
			return station.Name
		}
	}
	return ""
}

func FilterStations(stations []Station, query string) []Station {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return stations
	}

	var filtered []Station
	for _, station := range stations {
		if strings.Contains(strings.ToLower(station.Name), query) || strings.Contains(station.ID, query) {
			filtered = append(filtered, station)
		}
	}
	return filtered
}

func HTTPPostWebhookSchema(checkSegments bool) string {
	directSchema := `{
      "type": "object",
      "required": ["route_details", "departure_date", "fields"],
      "properties": {
        "route_details": {
          "type": "object",
          "required": [
            "priceFrom",
            "priceTo",
            "freeSeatsCount",
            "departureCityName",
            "arrivalCityName",
            "travelTime",
            "departureTime",
            "arrivalTime"
          ],
          "properties": {
            "priceFrom": { "type": "number" },
            "priceTo": { "type": "number" },
            "freeSeatsCount": { "type": "integer" },
            "departureCityName": { "type": "string" },
            "arrivalCityName": { "type": "string" },
            "travelTime": { "type": "string" },
            "departureTime": { "type": "string", "format": "date-time" },
            "arrivalTime": { "type": "string", "format": "date-time" }
          }
        },
        "departure_date": { "type": "string" },
        "fields": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name", "value", "inline"],
            "properties": {
              "name": { "type": "string" },
              "value": { "type": "string" },
              "inline": { "type": "boolean" }
            }
          }
        }
      }
    }`
	if !checkSegments {
		return directSchema
	}

	return `[
  ` + directSchema + `,
  {
      "type": "object",
      "required": ["route_info", "fields"],
      "properties": {
        "route_info": {
          "type": "object",
          "additionalProperties": { "type": "string" }
        },
        "fields": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name", "value", "inline"],
            "properties": {
              "name": { "type": "string" },
              "value": { "type": "string" },
              "inline": { "type": "boolean" }
            }
          }
        }
      }
    }
]`
}

func ErrorTitle(message string) string {
	switch {
	case strings.Contains(message, "Choose both stations"):
		return "Complete the search"
	case strings.Contains(message, "different stations"):
		return "Choose a different destination"
	case strings.Contains(message, "Webhook URL"):
		return "Webhook URL is missing"
	case strings.Contains(message, "Choose a route"):
		return "Choose a route"
	case strings.Contains(message, "watchdog"):
		return "Watchdog update failed"
	default:
		return "Could not load routes"
	}
}

func WebhookTypeLabel(webhookType string) string {
	if webhookType == "simple" {
		return "HTTP POST"
	}
	return "Discord"
}

func PriceRange(route models.Route, currency string) string {
	if currency == "" {
		currency = "CZK"
	}
	switch {
	case route.PriceFrom > 0 && route.PriceTo > 0:
		return fmt.Sprintf("%s - %s %s", FormatPrice(route.PriceFrom), FormatPrice(route.PriceTo), currency)
	case route.PriceFrom > 0:
		return fmt.Sprintf("from %s %s", FormatPrice(route.PriceFrom), currency)
	case route.PriceTo > 0:
		return fmt.Sprintf("to %s %s", FormatPrice(route.PriceTo), currency)
	default:
		return "No price"
	}
}

func FormatPrice(price float64) string {
	if price == float64(int64(price)) {
		return fmt.Sprintf("%.0f", price)
	}
	return fmt.Sprintf("%.2f", price)
}

func SeatText(route models.Route) string {
	if route.FreeSeats > 0 {
		return fmt.Sprintf("%d free seats", route.FreeSeats)
	}
	return "Sold out"
}

func routeRowClass(route models.Route) string {
	if route.Watchdog {
		return "route-row is-watchdog"
	}
	return "route-row"
}

func seatBadgeClass(route models.Route) string {
	if route.FreeSeats > 0 {
		return "badge badge-avail"
	}
	return "badge badge-empty"
}
