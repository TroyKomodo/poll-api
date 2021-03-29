package mongo

import (
	"context"

	"github.com/troydota/api.poll.komodohype.dev/configure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	log "github.com/sirupsen/logrus"
)

var Database *mongo.Database
var Ctx = context.TODO()

var ErrNoDocuments = mongo.ErrNoDocuments

func init() {
	clientOptions := options.Client().ApplyURI(configure.Config.GetString("mongo_uri"))
	client, err := mongo.Connect(Ctx, clientOptions)
	if err != nil {
		panic(err)
	}

	err = client.Ping(Ctx, nil)
	if err != nil {
		panic(err)
	}

	Database = client.Database(configure.Config.GetString("mongo_db"))

	_, err = Database.Collection("pollanswers").Indexes().CreateMany(Ctx, []mongo.IndexModel{
		{Keys: bson.M{"poll_id": 1}},
		{Keys: bson.M{"ip": 1}},
	})
	if err != nil {
		log.Errorf("mongodb, err=%v", err)
		return
	}
}
