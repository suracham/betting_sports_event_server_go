# Sports Events Betting Management Server

A REST API server that manages sporting events and their betting odds. External data providers push event data into the system in real time, and support teams can query that data through the same API.

---

## What does it do?

- **Store events** — save a sporting event (e.g. "Real Madrid vs Barcelona") with its markets and odds
- **Update odds** — change the odds for specific selections within an event without touching anything else
- **Look up events** — fetch a single event by ID, or search/sort all events
- **Delete events** — remove an event from the system

---

## Requirements

- [Go 1.22+](https://go.dev/dl/)
- [MongoDB](https://www.mongodb.com/try/download/community) running locally (or reachable over the network)

---

## Getting started

### 1. Clone the repo

```bash
git clone https://github.com/suracham/betting_sports_event_server_go.git
cd betting_sports_event_server_go
```

### 2. Download dependencies

```bash
go mod tidy
```

### 3. Start MongoDB

If you have MongoDB installed locally, start it with:

```bash
mongod
```

It runs on `127.0.0.1:27017` by default, which is what the server expects.

### 4. Build and run the server

```bash
go build -o server ./cmd/server
./server
```

The server starts on port **8080**. You should see:

```
[BET_SPORT_API] Server starting on 0.0.0.0:8080
```

### Available flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server-ip` | `0.0.0.0` | Network interface to listen on |
| `--server-port` | `8080` | Port to listen on |
| `--db-ip` | `127.0.0.1` | MongoDB host |
| `--db-port` | `27017` | MongoDB port |
| `--version` | — | Print version and exit |

Example with custom ports:

```bash
./server --server-port 1234 --db-ip 192.168.1.10
```

---

## API reference

All request and response bodies are JSON. All responses include a `status` field when returning a message.

### Create an event

**POST** `/api/match/createevent` — fails if the event already exists
**PUT** `/api/match/createevent` — creates or overwrites the event

```bash
curl -X POST http://localhost:8080/api/match/createevent \
  -H "Content-Type: application/json" \
  -d '{
    "id": 8661032861909884224,
    "message_type": "NewEvent",
    "event": {
      "id": 994839351740,
      "name": "Real Madrid vs Barcelona",
      "startTime": "2021-06-20 10:30:00",
      "sport": { "id": 221, "name": "Football" },
      "markets": [
        {
          "id": 385086549360973392,
          "name": "Winner",
          "selections": [
            { "id": 8243901714083343527, "name": "Real Madrid", "odds": 1.01 },
            { "id": 5737666888266680774, "name": "Barcelona",   "odds": 1.01 }
          ]
        }
      ]
    }
  }'
```

Response `200 OK`:
```json
{ "status": "Created Event" }
```

---

### Update odds

**PUT** `/api/match/updateodds`

Only the `odds` values you provide are changed. Everything else (event name, sport, market names, etc.) stays the same.

```bash
curl -X PUT http://localhost:8080/api/match/updateodds \
  -H "Content-Type: application/json" \
  -d '{
    "id": 8661032861909884224,
    "message_type": "UpdateOdds",
    "event": {
      "id": 994839351740,
      "markets": [
        {
          "id": 385086549360973392,
          "selections": [
            { "id": 8243901714083343527, "name": "Real Madrid", "odds": 10.00 },
            { "id": 5737666888266680774, "name": "Barcelona",   "odds": 5.55 }
          ]
        }
      ]
    }
  }'
```

Response `200 OK`:
```json
{ "status": "Updated Odds successfully" }
```

---

### Get a single event

**GET** `/api/match/{id}`

```bash
curl http://localhost:8080/api/match/994839351740
```

Returns the full event object, or `404` if not found.

---

### Search / list events

**GET** `/api/match/?<params>`

Filter by field:

```bash
# Find all events with this name
curl "http://localhost:8080/api/match/?name=Real+Madrid+vs+Barcelona"

# Find by sport
curl "http://localhost:8080/api/match/?sport=Football"
```

Sort all events:

```bash
# Sort by start time (earliest first)
curl "http://localhost:8080/api/match/?ordering=startTime"

# Sort by name or id
curl "http://localhost:8080/api/match/?ordering=name"
curl "http://localhost:8080/api/match/?ordering=id"
```

Returns a list of matching events with summary fields (`id`, `name`, `url`, `startTime`).

---

### Delete an event

**DELETE** `/api/match/deleteevent/{id}`

```bash
curl -X DELETE http://localhost:8080/api/match/deleteevent/994839351740
```

Response `200 OK`:
```json
{ "status": "Deleted Event" }
```

---

### Health check

```bash
curl http://localhost:8080/health
# OK
```

---

## Running with Docker

```bash
# Build the image
docker build -t betting-server .

# Run (connects to MongoDB on your host machine)
docker run -p 8080:8080 betting-server \
  --db-ip host.docker.internal --db-port 27017
```

---

## Migrating data from the Python server

If you were previously running the Python version of this server, your MongoDB data is stored in a format that the Go server cannot read directly. Run the migration tool once before starting the Go server:

```bash
go build -o migrate ./cmd/migrate

# Preview what will be changed (no writes)
./migrate --dry-run

# Apply the migration
./migrate --db-ip 127.0.0.1 --db-port 27017
```

The tool is safe to re-run — it skips documents that are already in the correct format.

---

## Running tests

```bash
go test ./...
```
