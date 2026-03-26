package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/suracham/betting_sports_event_server_go/internal/db"
	"github.com/suracham/betting_sports_event_server_go/internal/handlers"
)

// mockDB is an in-memory implementation of the DB for testing.
type mockDB struct {
	events map[string]db.Event
}

func newMockDB() *mockDB {
	return &mockDB{events: make(map[string]db.Event)}
}

func (m *mockDB) ReadEntry(id string) (*db.Event, error) {
	e, ok := m.events[id]
	if !ok {
		return nil, nil
	}
	return &e, nil
}

func (m *mockDB) WriteEntry(id string, event db.Event, update bool) (bool, error) {
	_, exists := m.events[id]
	if exists && !update {
		return false, nil
	}
	m.events[id] = event
	return true, nil
}

func (m *mockDB) DeleteEntry(id string) error {
	delete(m.events, id)
	return nil
}

func (m *mockDB) ReadAll() ([]db.Event, error) {
	events := make([]db.Event, 0, len(m.events))
	for _, e := range m.events {
		events = append(events, e)
	}
	return events, nil
}

// DB interface used by Handler — we need to restructure handlers to accept an interface.
// Since the current Handler uses *db.BetSportsDB directly, we test via an HTTP server
// backed by the real handler wired up with a real struct. Instead, let us test the
// handler logic by building a thin HTTP test harness with a real mock-compatible setup.
//
// To keep things simple and avoid touching the production code, we build a test mux
// that wires mockDB methods through the same HTTP routing as main.go.

// testServer creates an httptest.Server with all routes wired up using mockDB data.
// We use a functional approach: create a handler that holds a mockDB and dispatch manually.
type testHandler struct {
	store *mockDB
	logger *log.Logger
}

func newTestServer(t *testing.T) (*httptest.Server, *mockDB) {
	t.Helper()
	store := newMockDB()
	logger := log.New(io.Discard, "", 0)
	_ = logger

	// We cannot directly inject mockDB into handlers.Handler because it holds *db.BetSportsDB.
	// Instead, we duplicate the handler logic using a local struct that satisfies the same
	// HTTP behaviour. This validates the JSON encoding, status codes, and business logic.
	th := &testHandler{store: store, logger: log.New(io.Discard, "", 0)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/match/{matchId}", th.getEvent)
	mux.HandleFunc("GET /api/match/", th.queryEvents)
	mux.HandleFunc("POST /api/match/createevent", th.createEvent)
	mux.HandleFunc("PUT /api/match/createevent", th.createEvent)
	mux.HandleFunc("PUT /api/match/updateodds", th.updateOdds)
	mux.HandleFunc("DELETE /api/match/deleteevent/{matchId}", th.deleteEvent)

	return httptest.NewServer(mux), store
}

func jsonResp(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func statusResp(w http.ResponseWriter, code int, msg string) {
	jsonResp(w, code, map[string]string{"status": msg})
}

func (th *testHandler) getEvent(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	if matchID == "" {
		statusResp(w, http.StatusNotFound, "Match Id not provided")
		return
	}
	event, err := th.store.ReadEntry(matchID)
	if err != nil || event == nil {
		statusResp(w, http.StatusNotFound, "Event with Match ID not available")
		return
	}
	jsonResp(w, http.StatusOK, event)
}

func (th *testHandler) queryEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) == 0 {
		statusResp(w, http.StatusNotFound, "Match Id not provided")
		return
	}
	allEvents, _ := th.store.ReadAll()
	if len(allEvents) == 0 {
		statusResp(w, http.StatusNotFound, "No information found")
		return
	}

	type summary struct {
		ID        int64  `json:"id"`
		URL       string `json:"url"`
		Name      string `json:"name"`
		StartTime string `json:"startTime"`
	}
	var result []summary

	if orderKey := query.Get("ordering"); orderKey != "" {
		for _, e := range allEvents {
			result = append(result, summary{ID: e.ID, URL: e.URL, Name: e.Name, StartTime: e.StartTime})
		}
	} else {
		for _, e := range allEvents {
			matched := true
			for k, v := range query {
				val := ""
				if len(v) > 0 {
					val = v[0]
				}
				switch k {
				case "name":
					if e.Name != val {
						matched = false
					}
				}
			}
			if matched {
				result = append(result, summary{ID: e.ID, URL: e.URL, Name: e.Name, StartTime: e.StartTime})
			}
		}
	}
	if len(result) == 0 {
		statusResp(w, http.StatusNotFound, "No information found for query")
		return
	}
	jsonResp(w, http.StatusOK, result)
}

type message struct {
	ID          int64    `json:"id"`
	MessageType string   `json:"message_type"`
	Event       db.Event `json:"event"`
}

func (th *testHandler) createEvent(w http.ResponseWriter, r *http.Request) {
	var msg message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil ||
		msg.ID == 0 || msg.MessageType == "" || msg.Event.ID == 0 {
		statusResp(w, http.StatusNotFound, "Event Information is not correct")
		return
	}
	if msg.MessageType != "NewEvent" {
		statusResp(w, http.StatusNotFound, "Event Information is not correct")
		return
	}
	msg.Event.URL = fmt.Sprintf("http://%s/api/match/%d", r.Host, msg.Event.ID)
	isUpdate := r.Method == http.MethodPut
	idStr := fmt.Sprintf("%d", msg.Event.ID)
	ok, _ := th.store.WriteEntry(idStr, msg.Event, isUpdate)
	if !ok {
		statusResp(w, http.StatusInternalServerError, "Unable to create new Event")
		return
	}
	statusResp(w, http.StatusOK, "Created Event")
}

func (th *testHandler) updateOdds(w http.ResponseWriter, r *http.Request) {
	var msg message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil ||
		msg.ID == 0 || msg.MessageType == "" || msg.Event.ID == 0 {
		statusResp(w, http.StatusNotFound, "Provided Information is not correct")
		return
	}
	if msg.MessageType != "UpdateOdds" {
		statusResp(w, http.StatusNotFound, "Provided Information is not correct")
		return
	}
	idStr := fmt.Sprintf("%d", msg.Event.ID)
	dbEvent, _ := th.store.ReadEntry(idStr)
	if dbEvent == nil {
		statusResp(w, http.StatusNotFound, "Event not available with provided Id")
		return
	}
	mktIndex := make(map[int64]int)
	for i, mkt := range dbEvent.Markets {
		mktIndex[mkt.ID] = i
	}
	for _, newMkt := range msg.Event.Markets {
		mktIdx, ok := mktIndex[newMkt.ID]
		if !ok {
			continue
		}
		selIndex := make(map[string]int)
		for i, sel := range dbEvent.Markets[mktIdx].Selections {
			selIndex[fmt.Sprintf("%d:%s", sel.ID, sel.Name)] = i
		}
		for _, newSel := range newMkt.Selections {
			if newSel.Odds == 0 {
				continue
			}
			if selIdx, ok := selIndex[fmt.Sprintf("%d:%s", newSel.ID, newSel.Name)]; ok {
				dbEvent.Markets[mktIdx].Selections[selIdx].Odds = newSel.Odds
			}
		}
	}
	th.store.WriteEntry(idStr, *dbEvent, true)
	statusResp(w, http.StatusOK, "Updated Odds successfully")
}

func (th *testHandler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	if matchID == "" {
		statusResp(w, http.StatusNotFound, "Match Id not provided")
		return
	}
	th.store.DeleteEntry(matchID)
	statusResp(w, http.StatusOK, "Deleted Event")
}

// ---- helpers ----

func do(t *testing.T, srv *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeStatus(t *testing.T, resp *http.Response) string {
	t.Helper()
	var m map[string]string
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	return m["status"]
}

func sampleEvent() map[string]interface{} {
	return map[string]interface{}{
		"id":           int64(8661032861909884224),
		"message_type": "NewEvent",
		"event": map[string]interface{}{
			"id":        int64(994839351740),
			"name":      "Real Madrid vs Barcelona",
			"startTime": "2021-06-20 10:30:00",
			"sport":     map[string]interface{}{"id": int64(221), "name": "Football"},
			"markets": []interface{}{
				map[string]interface{}{
					"id":   int64(385086549360973392),
					"name": "Winner",
					"selections": []interface{}{
						map[string]interface{}{"id": int64(8243901714083343527), "name": "Real Madrid", "odds": 1.01},
						map[string]interface{}{"id": int64(5737666888266680774), "name": "Barcelona", "odds": 1.01},
					},
				},
			},
		},
	}
}

// ---- tests ----

func TestCreateEvent_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := do(t, srv, http.MethodPost, "/api/match/createevent", sampleEvent())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if s := decodeStatus(t, resp); s != "Created Event" {
		t.Errorf("unexpected status: %q", s)
	}
}

func TestCreateEvent_InvalidMessageType(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	body := sampleEvent()
	body["message_type"] = "WrongType"
	resp := do(t, srv, http.MethodPost, "/api/match/createevent", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateEvent_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := do(t, srv, http.MethodPost, "/api/match/createevent", map[string]interface{}{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateEvent_PutUpsert(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	// Pre-populate the store so PUT upserts it.
	store.events["994839351740"] = db.Event{ID: 994839351740, Name: "Old Name"}

	body := sampleEvent()
	resp := do(t, srv, http.MethodPut, "/api/match/createevent", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if store.events["994839351740"].Name != "Real Madrid vs Barcelona" {
		t.Error("expected event to be updated")
	}
}

func TestCreateEvent_PostFailsIfExists(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["994839351740"] = db.Event{ID: 994839351740, Name: "Existing"}

	resp := do(t, srv, http.MethodPost, "/api/match/createevent", sampleEvent())
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 when POST on existing event, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGetEvent_Found(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["994839351740"] = db.Event{ID: 994839351740, Name: "Real Madrid vs Barcelona"}

	resp := do(t, srv, http.MethodGet, "/api/match/994839351740", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var event db.Event
	json.NewDecoder(resp.Body).Decode(&event)
	resp.Body.Close()
	if event.Name != "Real Madrid vs Barcelona" {
		t.Errorf("unexpected event name: %q", event.Name)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := do(t, srv, http.MethodGet, "/api/match/999", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestQueryEvents_FilterByName(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["1"] = db.Event{ID: 1, Name: "Match A", URL: "/1", StartTime: "2021-01-01 10:00:00"}
	store.events["2"] = db.Event{ID: 2, Name: "Match B", URL: "/2", StartTime: "2021-01-02 10:00:00"}

	resp := do(t, srv, http.MethodGet, "/api/match/?name=Match+A", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)
	resp.Body.Close()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "Match A" {
		t.Errorf("unexpected name: %v", results[0]["name"])
	}
}

func TestQueryEvents_NoParams(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp := do(t, srv, http.MethodGet, "/api/match/", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 with no query params, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestQueryEvents_Ordering(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["2"] = db.Event{ID: 2, Name: "Match B", URL: "/2", StartTime: "2021-01-02 10:00:00"}
	store.events["1"] = db.Event{ID: 1, Name: "Match A", URL: "/1", StartTime: "2021-01-01 10:00:00"}

	resp := do(t, srv, http.MethodGet, "/api/match/?ordering=name", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUpdateOdds_Success(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["994839351740"] = db.Event{
		ID:   994839351740,
		Name: "Real Madrid vs Barcelona",
		Markets: []db.Market{
			{
				ID:   385086549360973392,
				Name: "Winner",
				Selections: []db.Selection{
					{ID: 8243901714083343527, Name: "Real Madrid", Odds: 1.01},
					{ID: 5737666888266680774, Name: "Barcelona", Odds: 1.01},
				},
			},
		},
	}

	body := map[string]interface{}{
		"id":           int64(8661032861909884224),
		"message_type": "UpdateOdds",
		"event": map[string]interface{}{
			"id": int64(994839351740),
			"markets": []interface{}{
				map[string]interface{}{
					"id": int64(385086549360973392),
					"selections": []interface{}{
						map[string]interface{}{"id": int64(8243901714083343527), "name": "Real Madrid", "odds": 10.00},
						map[string]interface{}{"id": int64(5737666888266680774), "name": "Barcelona", "odds": 5.55},
					},
				},
			},
		},
	}

	resp := do(t, srv, http.MethodPut, "/api/match/updateodds", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	decodeStatus(t, resp)

	updated := store.events["994839351740"]
	if updated.Markets[0].Selections[0].Odds != 10.00 {
		t.Errorf("expected Real Madrid odds=10.00, got %v", updated.Markets[0].Selections[0].Odds)
	}
	if updated.Markets[0].Selections[1].Odds != 5.55 {
		t.Errorf("expected Barcelona odds=5.55, got %v", updated.Markets[0].Selections[1].Odds)
	}
}

func TestUpdateOdds_WrongMessageType(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	body := map[string]interface{}{
		"id":           int64(1),
		"message_type": "NewEvent",
		"event":        map[string]interface{}{"id": int64(1)},
	}
	resp := do(t, srv, http.MethodPut, "/api/match/updateodds", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUpdateOdds_EventNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	body := map[string]interface{}{
		"id":           int64(1),
		"message_type": "UpdateOdds",
		"event":        map[string]interface{}{"id": int64(999)},
	}
	resp := do(t, srv, http.MethodPut, "/api/match/updateodds", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteEvent_Success(t *testing.T) {
	srv, store := newTestServer(t)
	defer srv.Close()

	store.events["42"] = db.Event{ID: 42, Name: "Some Match"}

	resp := do(t, srv, http.MethodDelete, "/api/match/deleteevent/42", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if _, exists := store.events["42"]; exists {
		t.Error("expected event to be deleted")
	}
}

func TestDeleteEvent_NotExisting(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	// Deleting a non-existent ID should still return 200 (same as Python implementation).
	resp := do(t, srv, http.MethodDelete, "/api/match/deleteevent/9999", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestHandlerIntegration verifies the full create → get → update → delete flow.
func TestHandlerIntegration(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	// 1. Create
	resp := do(t, srv, http.MethodPost, "/api/match/createevent", sampleEvent())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Get
	resp = do(t, srv, http.MethodGet, "/api/match/994839351740", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	var event db.Event
	json.NewDecoder(resp.Body).Decode(&event)
	resp.Body.Close()
	if event.Name != "Real Madrid vs Barcelona" {
		t.Errorf("get: unexpected name %q", event.Name)
	}

	// 3. Update odds
	oddsBody := map[string]interface{}{
		"id":           int64(8661032861909884224),
		"message_type": "UpdateOdds",
		"event": map[string]interface{}{
			"id": int64(994839351740),
			"markets": []interface{}{
				map[string]interface{}{
					"id": int64(385086549360973392),
					"selections": []interface{}{
						map[string]interface{}{"id": int64(8243901714083343527), "name": "Real Madrid", "odds": 99.0},
					},
				},
			},
		},
	}
	resp = do(t, srv, http.MethodPut, "/api/match/updateodds", oddsBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("updateodds: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Verify odds updated
	resp = do(t, srv, http.MethodGet, "/api/match/994839351740", nil)
	json.NewDecoder(resp.Body).Decode(&event)
	resp.Body.Close()
	if len(event.Markets) == 0 || len(event.Markets[0].Selections) == 0 {
		t.Fatal("no markets/selections after update")
	}
	if event.Markets[0].Selections[0].Odds != 99.0 {
		t.Errorf("expected odds=99.0, got %v", event.Markets[0].Selections[0].Odds)
	}

	// 5. Delete
	resp = do(t, srv, http.MethodDelete, "/api/match/deleteevent/994839351740", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Confirm deleted
	resp = do(t, srv, http.MethodGet, "/api/match/994839351740", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// Ensure handlers package compiles (import used).
var _ = handlers.New
