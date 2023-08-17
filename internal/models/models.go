package models

type TrainTicket struct {
	ID             string   `json:"id"`
	DepartureTime  string   `json:"departureTime"`
	ArrivalTime    string   `json:"arrivalTime"`
	FreeSeatsCount int      `json:"freeSeatsCount"`
	PriceFrom      float64  `json:"priceFrom"`
	PriceTo        float64  `json:"priceTo"`
	TravelTime     string   `json:"travelTime"`
	VehicleTypes   []string `json:"vehicleTypes"`
	TransfersCount int      `json:"transfersCount"`
}

type Response struct {
	Routes []TrainTicket `json:"routes"`
}

type FreeSeat struct {
	Index     int    `json:"index"`
	SeatClass string `json:"seatClass"`
}

type Vehicle struct {
	FreeSeats     []FreeSeat `json:"freeSeats"`
	SeatClasses   []string   `json:"seatClasses"`
	VehicleNumber int        `json:"vehicleNumber"`
}

type Section struct {
	SectionId int64     `json:"sectionId"`
	Vehicles  []Vehicle `json:"vehicles"`
}

type FreeSeatsResponse []Section

type FreeSeatsError struct {
	Message string `json:"message"`
}

type RouteDetails struct {
	PriceFrom         float64 `json:"priceFrom"`
	PriceTo           float64 `json:"priceTo"`
	FreeSeatsCount    int     `json:"freeSeatsCount"`
	DepartureCityName string  `json:"departureCityName"`
	ArrivalCityName   string  `json:"arrivalCityName"`
	TravelTime        string  `json:"travelTime"`
	DepartureTime     string  `json:"departureTime"`
	ArrivalTime       string  `json:"arrivalTime"`
}

type RouteDetailsResponse struct {
	PriceFrom         float64 `json:"priceFrom"`
	PriceTo           float64 `json:"priceTo"`
	FreeSeatsCount    int     `json:"freeSeatsCount"`
	DepartureCityName string  `json:"departureCityName"`
	ArrivalCityName   string  `json:"arrivalCityName"`
	DepartureTime     string  `json:"departureTime"`
	ArrivalTime       string  `json:"arrivalTime"`
	Sections          []struct {
		TravelTime string `json:"travelTime"`
	} `json:"sections"`
}
