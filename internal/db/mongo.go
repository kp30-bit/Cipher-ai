package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoClient wraps the mongo.Client
type MongoClient struct {
	*mongo.Client
}

// MongoDB wraps the database reference
type MongoDB struct {
	*mongo.Database
}

// InitMongo initializes a MongoDB client and returns a client & database reference
func InitMongo(uri, dbName string) (*MongoClient, *MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, nil, err
	}

	// Ping to ensure connection is established
	if err := client.Ping(ctx, nil); err != nil {
		return nil, nil, err
	}

	mongoClient := &MongoClient{Client: client}
	mongoDB := &MongoDB{Database: client.Database(dbName)}

	return mongoClient, mongoDB, nil
}

// Disconnect closes the MongoDB connection
func (c *MongoClient) Disconnect(ctx context.Context) error {
	return c.Client.Disconnect(ctx)
}
