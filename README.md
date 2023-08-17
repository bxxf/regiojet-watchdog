# RegioJet Watchdog
A service to monitor train routes and notify via Discord when seats are available.

## Prerequisites
- Golang (version 1.20 or higher)
- Redis server
- Discord webhook

## Setup
First, you will need to set up your environment variables. Create a file named .env in the root directory of this project and add the following line:
```REDIS_URL=your_redis_url```
Replace `your_redis_url` with your actual Redis connection URL.

## Running the Server
Navigate to the project directory and run the following command to start the server:
```go run .```
This will start the server, and it should be running at [http://localhost:7900](http://localhost:7900) by default.

## How to Use

### Step 1: Fetch Available Routes
Make a GET request to fetch available train routes based on `stationFromID`, `stationToID`, and `departureDate` parameters
For example:
```http://localhost:7900/routes?stationFromID=372825002&stationToID=1841058000&departureDate=18.08.2023```
This should return a response like this:
```json
[
    {
        "id": "6618452367",
        "departureTime": "08:12",
        "arrivalTime": "11:18",
        "priceFrom": 0,
        "priceTo": 0,
        "freeSeatsCount": 0
    },
    ...
]
```

To get IDs of all stations you can run 
```http://localhost:7900/constants```
This will return a JSON of stations with their IDS in this format:
```json
{
    "10204055": "Vienna - Schwechat Airport",
    "10204092": "Mosonmagyaróvár - nádraží",
    "1313142000": "Česká Třebová - Sta.",
    "1313142001": "Třinec centrum - Sta.",
    ...
}
```

From this output, note down the `id` of the desired route. This id is the `routeID` that will be used in the next step.

### Step 2: Set Up a Watchdog

To set up a watchdog that will periodically check for free seats on the chosen route and notify you via Discord when seats are available, make a POST request to:

```http://localhost:7900/watchdog```

with the following JSON payload:
```json
{
    "stationFromID": "372825002",
    "stationToID": "1841058000",
    "routeID": "6618452367",
    "webhookURL": "https://discord.com/api/webhooks/your_webhook_id/your_webhook_token"
}
```


Replace `your_webhook_id` and `your_webhook_token` with your actual Discord Webhook ID and token.

#### Discord Notification
Once a watchdog is set up, the service will periodically check the chosen route for free seats. When free seats are available, it will send a notification to the Discord channel associated with the provided Webhook URL.

## To Be Done

### Cancelling Running Watchdogs
In the future, an endpoint will be available to cancel currently running watchdogs. You will be able to send a request to this endpoint with the ID of the watchdog you want to cancel.

### UI for Creating New Watchdogs
A user-friendly interface is planned to simplify the process of creating new watchdogs. This UI will be accessible via a web browser and will provide a simple form to enter the necessary information to set up a new watchdog.

### Automatic reservations
A future feature is planned to enable automatic reservations of available seats. When the watchdog detects that seats on a specified route have become available, it will automatically reserve a seat. This feature will ensure that you never miss out on an available seat by automating the reservation process, and will include customizable options such as

## Contributing
Please feel free to open issues or submit pull requests.