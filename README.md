# RegioJet Watchdog
RegioJet Watchdog is a tool designed to help travelers secure their journey by keeping tabs on seat availability. This service constantly monitors specific train routes and sends notifications via Discord as soon as seats on those routes become available. Additionally, it can find alternative routes to help you reach your destination even when your primary option is fully booked.

## Features

### Free Seat Notifications
Once you set up a "watchdog", this service will continuously monitor a specified train route and check for available seats. When seats are found to be available, you will receive an instant notification via Discord, allowing you to book your seat as soon as possible.


### Alternative Routes Finder:
In case your primary route is fully booked, this service can suggest alternative options on tickets that involve switching your seat, but not the train itself. This service breaks down your journey into smaller segments between intermediate stations. For each segment, it checks for available seats and suggests options where you may need to switch your seat at certain stations. For example, if you are traveling from Station A to Station D, it might find availability from A to B, a different seat from B to C, and yet another seat from C to D, all on the same train.

<img src="https://github.com/bxxf/regiojet-watchdog/assets/43238984/d1adecf9-620c-4689-afa1-2f78a23a963d" width="300">

## Prerequisites
- Golang (version 1.20 or higher)
- Redis server
- Discord webhook

## Setup
First, you will need to set up your environment variables. Create a file named .env in the root directory of this project and add the following line:
```REDIS_URL=your_redis_url```
Replace `your_redis_url` with your actual Redis connection URL. You can also add `PORT`, if you want to change it from default `7900`.

## Running the Server
Navigate to the project directory and run the following command to start the server:
```go run .```
This will start the server, and it should be running at [http://localhost:7900](http://localhost:7900) by default.

## How to Use

### Step 1: Fetch Available Routes
Make a GET request to fetch available train routes based on `stationFromID`, `stationToID`, and `departureDate` parameters.

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

To get IDs of all stations you can run:
```http://localhost:7900/constants```

With response in this format:
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
A future feature is planned to enable automatic reservations of available seats. When the watchdog detects that seats on a specified route have become available, it will automatically reserve a seat. This feature will ensure that you never miss out on an available seat by automating the reservation process.ss

## Contributing
Please feel free to open issues or submit pull requests.
