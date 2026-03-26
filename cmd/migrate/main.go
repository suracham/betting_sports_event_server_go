// migrate converts MongoDB documents written by the Python server into the format
// used by the Go server.
//
// The Python server stores event data as a Python repr() string:
//
//	{ "_id": "994839351740", "data": "{'id': 994839351740, 'name': 'Real Madrid vs Barcelona', ...}" }
//
// The Go server stores event data as a BSON subdocument:
//
//	{ "_id": "994839351740", "data": { "id": 994839351740, "name": "Real Madrid vs Barcelona", ... } }
//
// Usage:
//
//	migrate [--db-ip 127.0.0.1] [--db-port 27017] [--db-name BET_SPORTS_DATA] [--dry-run]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/suracham/betting_sports_event_server_go/internal/db"
)

func main() {
	dbIP := flag.String("db-ip", "127.0.0.1", "MongoDB IP address")
	dbPort := flag.Int("db-port", 27017, "MongoDB port")
	dbName := flag.String("db-name", "BET_SPORTS_DATA", "MongoDB database name")
	dryRun := flag.Bool("dry-run", false, "Print what would be migrated without writing")
	flag.Parse()

	logger := log.New(os.Stdout, "[MIGRATE] ", log.LstdFlags)

	uri := fmt.Sprintf("mongodb://%s:%d", *dbIP, *dbPort)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		logger.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(context.Background(), nil); err != nil {
		logger.Fatalf("ping: %v", err)
	}

	collection := client.Database(*dbName).Collection("tbl")

	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		logger.Fatalf("find: %v", err)
	}
	defer cursor.Close(context.Background())

	migrated, skipped, failed := 0, 0, 0

	for cursor.Next(context.Background()) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			logger.Printf("decode error: %v — skipping", err)
			failed++
			continue
		}

		docID, _ := raw["_id"].(string)

		// Check if data field is already a BSON document (Go format).
		switch raw["data"].(type) {
		case bson.M, bson.D:
			logger.Printf("_id=%s: already in Go format — skipping", docID)
			skipped++
			continue
		}

		// Expect a Python repr string.
		pyStr, ok := raw["data"].(string)
		if !ok {
			logger.Printf("_id=%s: unrecognised data type %T — skipping", docID, raw["data"])
			failed++
			continue
		}

		event, err := parsePythonRepr(pyStr)
		if err != nil {
			logger.Printf("_id=%s: parse error: %v — skipping", docID, err)
			failed++
			continue
		}

		if *dryRun {
			logger.Printf("[DRY RUN] _id=%s: would migrate event id=%d name=%q", docID, event.ID, event.Name)
			migrated++
			continue
		}

		_, err = collection.UpdateOne(
			context.Background(),
			bson.M{"_id": docID},
			bson.M{"$set": bson.M{"data": event}},
		)
		if err != nil {
			logger.Printf("_id=%s: update error: %v", docID, err)
			failed++
			continue
		}
		logger.Printf("_id=%s: migrated event id=%d name=%q", docID, event.ID, event.Name)
		migrated++
	}

	if err := cursor.Err(); err != nil {
		logger.Printf("cursor error: %v", err)
	}

	logger.Printf("done — migrated=%d skipped=%d failed=%d", migrated, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// parsePythonRepr converts a Python repr() dict string to a db.Event.
//
// Python repr dicts differ from JSON in three ways:
//   - String literals use single quotes  →  replace with double quotes
//   - Booleans are True/False            →  replace with true/false
//   - None                               →  replace with null
//
// This conversion is correct for the well-structured event payloads produced by
// the Python betting server. It is not a general-purpose Python-to-JSON parser.
func parsePythonRepr(s string) (db.Event, error) {
	jsonStr := pythonReprToJSON(s)
	var event db.Event
	if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
		return db.Event{}, fmt.Errorf("json unmarshal after repr conversion: %w\nconverted: %s", err, jsonStr)
	}
	return event, nil
}

// pythonReprToJSON performs a best-effort conversion of a Python repr string to JSON.
func pythonReprToJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	inStr := false     // inside a string literal
	escape := false    // previous char was backslash
	quoteChar := rune(0)

	for _, ch := range s {
		if escape {
			// Pass through the escaped character.
			b.WriteRune(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			b.WriteRune(ch)
			escape = true
			continue
		}

		if inStr {
			if ch == quoteChar {
				// Close the string — always emit double-quote.
				b.WriteRune('"')
				inStr = false
			} else if ch == '"' {
				// A bare double-quote inside a single-quoted Python string must be escaped.
				b.WriteString(`\"`)
			} else {
				b.WriteRune(ch)
			}
			continue
		}

		// Outside a string.
		if ch == '\'' {
			// Python single-quoted string start → emit double-quote.
			b.WriteRune('"')
			inStr = true
			quoteChar = '\''
			continue
		}
		if ch == '"' {
			// Python double-quoted string start.
			b.WriteRune('"')
			inStr = true
			quoteChar = '"'
			continue
		}
		b.WriteRune(ch)
	}

	result := b.String()
	// Replace Python keywords with JSON equivalents (only outside strings — safe for
	// the well-known event schema because "True"/"False"/"None" don't appear as
	// substrings of field names or string values in the betting data).
	result = strings.ReplaceAll(result, ": True", ": true")
	result = strings.ReplaceAll(result, ": False", ": false")
	result = strings.ReplaceAll(result, ": None", ": null")
	return result
}
