package db

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Data models

type Sport struct {
	ID   int64  `json:"id" bson:"id"`
	Name string `json:"name" bson:"name"`
}

type Selection struct {
	ID   int64   `json:"id" bson:"id"`
	Name string  `json:"name" bson:"name"`
	Odds float64 `json:"odds" bson:"odds"`
}

type Market struct {
	ID         int64       `json:"id" bson:"id"`
	Name       string      `json:"name" bson:"name"`
	Selections []Selection `json:"selections" bson:"selections"`
}

type Event struct {
	ID        int64    `json:"id" bson:"id"`
	Name      string   `json:"name" bson:"name"`
	StartTime string   `json:"startTime" bson:"startTime"`
	URL       string   `json:"url" bson:"url"`
	Sport     Sport    `json:"sport" bson:"sport"`
	Markets   []Market `json:"markets" bson:"markets"`
}

type EventSummary struct {
	ID        int64  `json:"id"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
}

// BetSportsDB is the MongoDB interface for sports betting data.
type BetSportsDB struct {
	collection *mongo.Collection
	client     *mongo.Client
	logger     *log.Logger
}

func NewBetSportsDB(host string, port int, dbName string, logger *log.Logger) (*BetSportsDB, error) {
	uri := fmt.Sprintf("mongodb://%s:%d", host, port)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	collection := client.Database(dbName).Collection("tbl")
	return &BetSportsDB{
		collection: collection,
		client:     client,
		logger:     logger,
	}, nil
}

// fixID sanitizes an ID string for use as a MongoDB document _id.
func (db *BetSportsDB) fixID(id string) string {
	id = strings.ReplaceAll(id, ":", "_")
	id = strings.ReplaceAll(id, ".", "_")
	id = strings.ReplaceAll(id, "/", "_")
	return strings.Join(strings.Fields(id), "")
}

func (db *BetSportsDB) ReadEntry(clID string) (*Event, error) {
	id := db.fixID(clID)
	var result struct {
		Data Event `bson:"data"`
	}
	err := db.collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&result)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		db.logger.Printf("Could not get data for ID: %s, error: %v", id, err)
		return nil, err
	}
	return &result.Data, nil
}

// WriteEntry inserts or updates an event. If update=false and the entry exists, returns (false, nil).
func (db *BetSportsDB) WriteEntry(clID string, event Event, update bool) (bool, error) {
	id := db.fixID(clID)

	var existing bson.M
	err := db.collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&existing)

	if err == nil {
		if !update {
			return false, nil
		}
		_, err = db.collection.UpdateOne(
			context.Background(),
			bson.M{"_id": id},
			bson.M{"$set": bson.M{"data": event}},
		)
		if err != nil {
			db.logger.Printf("Could not update data for ID: %s, error: %v", id, err)
			return false, err
		}
	} else if err == mongo.ErrNoDocuments {
		_, err = db.collection.InsertOne(context.Background(), bson.M{"_id": id, "data": event})
		if err != nil {
			db.logger.Printf("Could not insert data for ID: %s, error: %v", id, err)
			return false, err
		}
	} else {
		return false, err
	}
	return true, nil
}

func (db *BetSportsDB) DeleteEntry(clID string) error {
	id := db.fixID(clID)
	_, err := db.collection.DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		db.logger.Printf("Could not delete entry for ID: %s, error: %v", id, err)
		return err
	}
	return nil
}

func (db *BetSportsDB) ReadAll() ([]Event, error) {
	cursor, err := db.collection.Find(context.Background(), bson.M{})
	if err != nil {
		db.logger.Printf("Could not read all entries, error: %v", err)
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []struct {
		Data Event `bson:"data"`
	}
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(results))
	for _, r := range results {
		events = append(events, r.Data)
	}
	return events, nil
}
