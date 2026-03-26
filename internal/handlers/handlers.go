package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/suracham/betting_sports_event_server_go/internal/db"
)

const timeLayout = "2006-01-02 15:04:05"

// Message is the top-level request body structure.
type Message struct {
	ID          int64    `json:"id"`
	MessageType string   `json:"message_type"`
	Event       db.Event `json:"event"`
}

type Handler struct {
	db     *db.BetSportsDB
	logger *log.Logger
}

func New(database *db.BetSportsDB, logger *log.Logger) *Handler {
	return &Handler{db: database, logger: logger}
}

func jsonResponse(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func statusResponse(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"status": msg})
}

// GetEvent handles GET /api/match/{matchId}
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	if matchID == "" {
		statusResponse(w, http.StatusNotFound, "Match Id not provided")
		return
	}

	event, err := h.db.ReadEntry(matchID)
	if err != nil {
		statusResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if event == nil {
		statusResponse(w, http.StatusNotFound, "Event with Match ID not available")
		return
	}
	jsonResponse(w, http.StatusOK, event)
}

// QueryEvents handles GET /api/match/ with optional query parameters.
// Supports ?ordering=startTime|id|name and arbitrary field filters.
func (h *Handler) QueryEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) == 0 {
		statusResponse(w, http.StatusNotFound, "Match Id not provided")
		return
	}

	allEvents, err := h.db.ReadAll()
	if err != nil || len(allEvents) == 0 {
		statusResponse(w, http.StatusNotFound, "No information found")
		return
	}

	var result []db.EventSummary

	if orderKey := query.Get("ordering"); orderKey != "" {
		if orderKey != "startTime" && orderKey != "id" && orderKey != "name" {
			statusResponse(w, http.StatusNotFound, "Ordering can't be done with provided information")
			return
		}
		result = orderEvents(allEvents, orderKey)
	} else {
		queryMap := make(map[string]string, len(query))
		for k, v := range query {
			if len(v) > 0 {
				queryMap[k] = v[0]
			}
		}
		result = filterEvents(allEvents, queryMap)
	}

	if len(result) == 0 {
		statusResponse(w, http.StatusNotFound, fmt.Sprintf("No information found for query %v", query))
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

func filterEvents(events []db.Event, query map[string]string) []db.EventSummary {
	var matched []db.EventSummary
	for _, evnt := range events {
		if matchesQuery(evnt, query) {
			matched = append(matched, db.EventSummary{
				ID:        evnt.ID,
				URL:       evnt.URL,
				Name:      evnt.Name,
				StartTime: evnt.StartTime,
			})
		}
	}
	return matched
}

func matchesQuery(evnt db.Event, query map[string]string) bool {
	for key, val := range query {
		switch key {
		case "id":
			if strconv.FormatInt(evnt.ID, 10) != val {
				return false
			}
		case "name":
			if evnt.Name != val {
				return false
			}
		case "startTime":
			if evnt.StartTime != val {
				return false
			}
		case "sport":
			if evnt.Sport.Name != val {
				return false
			}
		}
	}
	return true
}

func orderEvents(events []db.Event, key string) []db.EventSummary {
	sorted := make([]db.Event, len(events))
	copy(sorted, events)

	switch key {
	case "startTime":
		sort.Slice(sorted, func(i, j int) bool {
			ti, _ := time.Parse(timeLayout, sorted[i].StartTime)
			tj, _ := time.Parse(timeLayout, sorted[j].StartTime)
			return ti.Before(tj)
		})
	case "id":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].ID < sorted[j].ID
		})
	case "name":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
	}

	result := make([]db.EventSummary, 0, len(sorted))
	for _, evnt := range sorted {
		result = append(result, db.EventSummary{
			ID:        evnt.ID,
			URL:       evnt.URL,
			Name:      evnt.Name,
			StartTime: evnt.StartTime,
		})
	}
	return result
}

// CreateEvent handles POST /api/match/createevent and PUT /api/match/createevent.
// PUT allows overwriting an existing event; POST does not.
func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		statusResponse(w, http.StatusNotFound, "Event Information is not correct")
		return
	}

	if msg.ID == 0 || msg.MessageType == "" || msg.Event.ID == 0 {
		statusResponse(w, http.StatusNotFound, "Event Information is not correct")
		return
	}
	if msg.MessageType != "NewEvent" {
		statusResponse(w, http.StatusNotFound, "Event Information is not correct")
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	msg.Event.URL = fmt.Sprintf("%s://%s/api/match/%d", scheme, r.Host, msg.Event.ID)

	isUpdate := r.Method == http.MethodPut
	idStr := strconv.FormatInt(msg.Event.ID, 10)
	ok, err := h.db.WriteEntry(idStr, msg.Event, isUpdate)
	if err != nil {
		h.logger.Printf("WriteEntry error: %v", err)
		statusResponse(w, http.StatusInternalServerError, "Unable to create new Event")
		return
	}
	if !ok {
		statusResponse(w, http.StatusInternalServerError, "Unable to create new Event")
		return
	}
	statusResponse(w, http.StatusOK, "Created Event")
}

// UpdateOdds handles PUT /api/match/updateodds.
// Only updates odds for matching selections; all other event data is unchanged.
func (h *Handler) UpdateOdds(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		statusResponse(w, http.StatusNotFound, "Provided Information is not correct")
		return
	}

	if msg.ID == 0 || msg.MessageType == "" || msg.Event.ID == 0 {
		statusResponse(w, http.StatusNotFound, "Provided Information is not correct")
		return
	}
	if msg.MessageType != "UpdateOdds" {
		statusResponse(w, http.StatusNotFound, "Provided Information is not correct")
		return
	}

	idStr := strconv.FormatInt(msg.Event.ID, 10)
	dbEvent, err := h.db.ReadEntry(idStr)
	if err != nil || dbEvent == nil {
		statusResponse(w, http.StatusNotFound, "Event not available with provided Id")
		return
	}

	// Index DB markets by ID for O(1) lookup.
	mktIndex := make(map[int64]int)
	for i, mkt := range dbEvent.Markets {
		mktIndex[mkt.ID] = i
	}

	for _, newMkt := range msg.Event.Markets {
		mktIdx, ok := mktIndex[newMkt.ID]
		if !ok {
			continue
		}
		// Index existing selections by "id:name".
		selIndex := make(map[string]int)
		for i, sel := range dbEvent.Markets[mktIdx].Selections {
			selIndex[fmt.Sprintf("%d:%s", sel.ID, sel.Name)] = i
		}
		for _, newSel := range newMkt.Selections {
			if newSel.Odds == 0 {
				continue
			}
			key := fmt.Sprintf("%d:%s", newSel.ID, newSel.Name)
			if selIdx, ok := selIndex[key]; ok {
				dbEvent.Markets[mktIdx].Selections[selIdx].Odds = newSel.Odds
			}
		}
	}

	ok, err := h.db.WriteEntry(idStr, *dbEvent, true)
	if err != nil || !ok {
		statusResponse(w, http.StatusInternalServerError, "Unable to update Odds")
		return
	}
	statusResponse(w, http.StatusOK, "Updated Odds successfully")
}

// DeleteEvent handles DELETE /api/match/deleteevent/{matchId}
func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	if matchID == "" {
		statusResponse(w, http.StatusNotFound, "Match Id not provided")
		return
	}

	if err := h.db.DeleteEntry(matchID); err != nil {
		statusResponse(w, http.StatusInternalServerError, "Unable to delete event")
		return
	}
	statusResponse(w, http.StatusOK, "Deleted Event")
}
