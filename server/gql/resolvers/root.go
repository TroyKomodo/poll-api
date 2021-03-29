package resolvers

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/selection"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	"github.com/troydota/api.poll.komodohype.dev/mongo"
	"github.com/troydota/api.poll.komodohype.dev/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	errInternalServer = fmt.Errorf("internal server error")
	errMissingPoll    = fmt.Errorf("we don't know what poll that is")
)

type PollVote []int32

func New() *RootResolver {
	rr := &RootResolver{
		mtx:    &sync.Mutex{},
		subs:   make(map[string][]chan PollVote),
		pubsub: redis.Client.Subscribe(redis.Ctx),
	}

	go func() {
		ch := rr.pubsub.Channel()
		for {
			msg := <-ch
			vote := PollVote{}
			err := json.UnmarshalFromString(msg.Payload, &vote)
			if err != nil {
				log.Errorf("redis, err=%v", err)
				continue
			}
			wg := sync.WaitGroup{}
			rr.mtx.Lock()
			if v, ok := rr.subs[msg.Channel]; ok {
				wg.Add(len(v))
				for _, c := range v {
					go func(c chan PollVote) {
						defer wg.Done()
						defer recover()
						c <- vote
					}(c)
				}
			}
			wg.Wait()
			rr.mtx.Unlock()
		}
	}()

	return rr
}

type RootResolver struct {
	mtx    *sync.Mutex
	subs   map[string][]chan PollVote
	pubsub *redis.PubSub
}

func filterSlice(s []chan PollVote, r chan PollVote) []chan PollVote {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func (r *RootResolver) subscribe(event string, ch chan PollVote) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if v, ok := r.subs[event]; ok {
		r.subs[event] = append(v, ch)
	} else {
		r.subs[event] = []chan PollVote{ch}
		return r.pubsub.Subscribe(redis.Ctx, event)
	}
	return nil
}

func (r *RootResolver) unsubscribe(event string, ch chan PollVote) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	new := filterSlice(r.subs[event], ch)
	if len(new) == 0 {
		delete(r.subs, event)
		return r.pubsub.Unsubscribe(redis.Ctx, event)
	}
	r.subs[event] = new
	return nil
}

type selectedField struct {
	name     string
	children map[string]*selectedField
}

func generateSelectedFieldMap(ctx context.Context) *selectedField {
	var loop func(fields []*selection.SelectedField) map[string]*selectedField
	loop = func(fields []*selection.SelectedField) map[string]*selectedField {
		if len(fields) == 0 {
			return nil
		}
		m := map[string]*selectedField{}
		for _, f := range fields {
			m[f.Name] = &selectedField{
				name:     f.Name,
				children: loop(f.SelectedFields),
			}
		}
		return m
	}
	children := loop(graphql.SelectedFieldsFromContext(ctx))
	return &selectedField{
		name:     "query",
		children: children,
	}
}

func fetchPoll(id primitive.ObjectID, field *selectedField) (*mongo.Poll, error) {
	pipe := redis.Client.Pipeline()

	redisKey := fmt.Sprintf("cached:polls:%s", id.Hex())

	fetchVotes := false

	if field != nil {
		if v, ok := field.children["options"]; ok {
			if _, ok := v.children["votes"]; ok {
				fetchVotes = true
			}
		}
	}

	valCmd := pipe.Get(redis.Ctx, redisKey)
	var votesCmd *redis.StringStringMapCmd
	if fetchVotes {
		votesCmd = pipe.HGetAll(redis.Ctx, fmt.Sprintf("poll:votes:%s:options", id.Hex()))
	}
	_, err := pipe.Exec(redis.Ctx)
	if err != nil && err != redis.ErrNil {
		log.Errorf("redis, err=%v", err)
		return nil, errInternalServer
	}

	val, err := valCmd.Result()
	if err != nil && err != redis.ErrNil {
		log.Errorf("redis, err=%v", err)
		return nil, errInternalServer
	}

	if val == "dead" {
		return nil, nil
	}

	poll := &mongo.Poll{}
	if err == redis.ErrNil {
		result := mongo.Database.Collection("polls").FindOne(mongo.Ctx, bson.M{
			"_id": id,
		})
		err = result.Err()
		if err == mongo.ErrNoDocuments {
			if err = redis.Client.Set(redis.Ctx, redisKey, "dead", time.Hour*6).Err(); err != nil {
				log.Errorf("redis, err=%v", err)
			}
			return nil, nil
		}
		if err == nil {
			result.Decode(poll)
		}
		if err != nil {
			log.Errorf("mongo, err=%v", err)
			return nil, errInternalServer
		}

		pollStr, err := json.MarshalToString(poll)
		if err == nil {
			if err = redis.Client.Set(redis.Ctx, redisKey, pollStr, time.Hour*6).Err(); err != nil {
				log.Errorf("redis, err=%v", err)
			}
		} else {
			log.Errorf("redis, err=%v", err)
		}
	} else if err = json.UnmarshalFromString(val, poll); err != nil {
		log.Errorf("json, err=%v", err)
		return nil, errInternalServer
	}

	if field != nil {
		if _, ok := field.children["options"]; ok {
			var votes map[string]string
			if fetchVotes {
				votes, err = votesCmd.Result()
				if err != nil {
					if err == redis.ErrNil {
						votes = nil
					} else {
						log.Errorf("redis, err=%v", err)
						return nil, errInternalServer
					}
				}
			}
			options := make([]mongo.PollOption, len(poll.OptionsRaw))

			for i, v := range poll.OptionsRaw {
				var voteCount int32
				if votes != nil {
					if count, ok := votes[fmt.Sprint(i)]; ok {
						tVal, err := strconv.ParseInt(count, 10, 32)
						if err != nil {
							return nil, err
						}
						voteCount = int32(tVal)
					}
				}
				options[i] = mongo.PollOption{
					Title: v,
					Votes: voteCount,
				}
			}
			poll.Options = &options
		}
	}

	return poll, nil
}
