package redis

import (
	"context"

	"github.com/go-redis/redis/v8"
	"github.com/troydota/api.poll.komodohype.dev/configure"
)

var Ctx = context.Background()

var Client *redis.Client

func init() {
	options, err := redis.ParseURL(configure.Config.GetString("redis_uri"))
	if err != nil {
		panic(err)
	}

	Client = redis.NewClient(options)
}

type Message = redis.Message

const ErrNil = redis.Nil

type StringCmd = redis.StringCmd

type Pipeliner = redis.Pipeliner

type StringStringMapCmd = redis.StringStringMapCmd

type PubSub = redis.PubSub
