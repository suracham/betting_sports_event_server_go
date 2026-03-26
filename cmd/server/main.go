package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/suracham/betting_sports_event_server_go/internal/db"
	"github.com/suracham/betting_sports_event_server_go/internal/handlers"
)

const version = "1.0"

func main() {
	serverIP := flag.String("server-ip", "0.0.0.0", "IP to bind the server to")
	serverPort := flag.Int("server-port", 8080, "Port to bind the server to")
	dbIP := flag.String("db-ip", "127.0.0.1", "MongoDB IP address")
	dbPort := flag.Int("db-port", 27017, "MongoDB port")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	logger := log.New(os.Stdout, "[BET_SPORT_API] ", log.LstdFlags)

	database, err := db.NewBetSportsDB(*dbIP, *dbPort, "BET_SPORTS_DATA", logger)
	if err != nil {
		logger.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	h := handlers.New(database, logger)
	mux := http.NewServeMux()

	// Event retrieval
	mux.HandleFunc("GET /api/match/{matchId}", h.GetEvent)
	mux.HandleFunc("GET /api/match/", h.QueryEvents)

	// Event creation (POST = fail if exists, PUT = upsert)
	mux.HandleFunc("POST /api/match/createevent", h.CreateEvent)
	mux.HandleFunc("PUT /api/match/createevent", h.CreateEvent)

	// Odds update
	mux.HandleFunc("PUT /api/match/updateodds", h.UpdateOdds)

	// Event deletion
	mux.HandleFunc("DELETE /api/match/deleteevent/{matchId}", h.DeleteEvent)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf("%s:%d", *serverIP, *serverPort)
	logger.Printf("Server starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Fatalf("Server failed: %v", err)
	}
}
