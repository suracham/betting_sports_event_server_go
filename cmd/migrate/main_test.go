package main

import (
	"testing"
)

func TestPythonReprToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON string
	}{
		{
			name:     "simple dict",
			input:    "{'id': 1, 'name': 'foo'}",
			wantJSON: `{"id": 1, "name": "foo"}`,
		},
		{
			name:     "nested dict",
			input:    "{'sport': {'id': 221, 'name': 'Football'}}",
			wantJSON: `{"sport": {"id": 221, "name": "Football"}}`,
		},
		{
			name:     "list of dicts",
			input:    "{'markets': [{'id': 1, 'name': 'Winner'}]}",
			wantJSON: `{"markets": [{"id": 1, "name": "Winner"}]}`,
		},
		{
			name:     "float value",
			input:    "{'odds': 1.01}",
			wantJSON: `{"odds": 1.01}`,
		},
		{
			name:     "boolean values",
			input:    "{'active': True, 'deleted': False}",
			wantJSON: `{"active": true, "deleted": false}`,
		},
		{
			name:     "none value",
			input:    "{'result': None}",
			wantJSON: `{"result": null}`,
		},
		{
			name:     "string with double quotes inside single-quoted string",
			input:    `{'desc': 'say "hello"'}`,
			wantJSON: `{"desc": "say \"hello\""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pythonReprToJSON(tt.input)
			if got != tt.wantJSON {
				t.Errorf("\ngot:  %s\nwant: %s", got, tt.wantJSON)
			}
		})
	}
}

func TestParsePythonRepr(t *testing.T) {
	pyRepr := "{'id': 994839351740, 'name': 'Real Madrid vs Barcelona', " +
		"'startTime': '2021-06-20 10:30:00', 'url': 'http://localhost/api/match/994839351740', " +
		"'sport': {'id': 221, 'name': 'Football'}, " +
		"'markets': [{'id': 385086549360973392, 'name': 'Winner', " +
		"'selections': [{'id': 8243901714083343527, 'name': 'Real Madrid', 'odds': 1.01}, " +
		"{'id': 5737666888266680774, 'name': 'Barcelona', 'odds': 1.01}]}]}"

	event, err := parsePythonRepr(pyRepr)
	if err != nil {
		t.Fatalf("parsePythonRepr error: %v", err)
	}

	if event.ID != 994839351740 {
		t.Errorf("ID: got %d, want 994839351740", event.ID)
	}
	if event.Name != "Real Madrid vs Barcelona" {
		t.Errorf("Name: got %q, want %q", event.Name, "Real Madrid vs Barcelona")
	}
	if event.StartTime != "2021-06-20 10:30:00" {
		t.Errorf("StartTime: got %q", event.StartTime)
	}
	if event.Sport.Name != "Football" {
		t.Errorf("Sport.Name: got %q", event.Sport.Name)
	}
	if len(event.Markets) != 1 {
		t.Fatalf("Markets: got %d, want 1", len(event.Markets))
	}
	if len(event.Markets[0].Selections) != 2 {
		t.Fatalf("Selections: got %d, want 2", len(event.Markets[0].Selections))
	}
	if event.Markets[0].Selections[0].Odds != 1.01 {
		t.Errorf("Odds: got %v, want 1.01", event.Markets[0].Selections[0].Odds)
	}
}
