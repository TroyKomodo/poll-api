package resolvers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/troydota/api.poll.komodohype.dev/mongo"
	"github.com/troydota/api.poll.komodohype.dev/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (*RootResolver) Poll(ctx context.Context, args struct{ ID string }) (*pollResolver, error) {
	field := generateSelectedFieldMap(ctx)

	id, err := primitive.ObjectIDFromHex(args.ID)
	if err != nil {
		return nil, nil
	}

	poll, err := fetchPoll(id, field)
	if err != nil {
		return nil, err
	}
	if poll == nil {
		return nil, nil
	}

	return &pollResolver{poll, field}, nil
}

func (*RootResolver) Draft(ctx context.Context, args struct{ ID string }) (*draftResolver, error) {
	id, err := primitive.ObjectIDFromHex(args.ID)
	if err != nil {
		return nil, nil
	}

	redisKey := fmt.Sprintf("cached:drafts:%s", id.Hex())

	val, err := redis.Client.Get(redis.Ctx, redisKey).Result()
	if err != nil && err != redis.ErrNil {
		log.Errorf("redis, err=%v", err)
		return nil, errInternalServer
	}

	if val == "dead" {
		return nil, nil
	}

	draft := &mongo.Draft{}
	if err == redis.ErrNil {
		result := mongo.Database.Collection("drafts").FindOne(mongo.Ctx, bson.M{
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
			err = result.Decode(draft)
		}
		if err != nil {
			log.Errorf("mongo, err=%v", err)
			return nil, errInternalServer
		}

		draftStr, err := json.MarshalToString(draft)
		if err == nil {
			if err = redis.Client.Set(redis.Ctx, redisKey, draftStr, time.Hour*6).Err(); err != nil {
				log.Errorf("redis, err=%v", err)
			}
		} else {
			log.Errorf("redis, err=%v", err)
		}
	} else if err = json.UnmarshalFromString(val, draft); err != nil {
		log.Errorf("json, err=%v", err)
		return nil, errInternalServer
	}

	if draft == nil {
		return nil, nil
	}

	return &draftResolver{draft}, nil
}

type pollResolver struct {
	poll  *mongo.Poll
	field *selectedField
}

func (r *pollResolver) ID() string {
	return r.poll.ID.Hex()
}

func (r *pollResolver) Title() string {
	return r.poll.Title
}

func (r *pollResolver) Options() ([]mongo.PollOption, error) {
	if r.poll.Options == nil {
		var votes map[string]string
		var err error
		options := make([]mongo.PollOption, len(r.poll.OptionsRaw))
		if r.field != nil {
			if _, ok := r.field.children["options"].children["votes"]; ok {
				votes, err = redis.Client.HGetAll(redis.Ctx, fmt.Sprintf("poll:votes:%s:options", r.poll.ID.Hex())).Result()
				if err != nil {
					if err == redis.ErrNil {
						votes = nil
					} else {
						log.Errorf("redis, err=%v", err)
						return nil, errInternalServer
					}
				}
			}
		}

		for i, v := range r.poll.OptionsRaw {
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
		r.poll.Options = &options
	}
	return *r.poll.Options, nil
}

func (r *pollResolver) CheckIP() bool {
	return r.poll.CheckIP
}

func (r *pollResolver) MultiAnswer() bool {
	return r.poll.MultiAnswer
}

func (r *pollResolver) Expiry() *string {
	if r.poll.Expiry == nil {
		return nil
	}
	s := r.poll.Expiry.Format(time.RFC3339)
	return &s
}

func (r *pollResolver) CreatedAt() string {
	return r.poll.ID.Timestamp().Format(time.RFC3339)
}

type draftResolver struct {
	draft *mongo.Draft
}

func (r *draftResolver) ID() string {
	return r.draft.ID.Hex()
}

func (r *draftResolver) Title() string {
	return r.draft.Title
}

func (r *draftResolver) Options() []string {
	return r.draft.Options
}

func (r *draftResolver) CheckIP() bool {
	return r.draft.CheckIP
}

func (r *draftResolver) MultiAnswer() bool {
	return r.draft.MultiAnswer
}

func (r *draftResolver) Expiry() *int32 {
	return r.draft.Expiry
}

func (r *draftResolver) CreatedAt() string {
	return r.draft.ID.Timestamp().Format(time.RFC3339)
}
