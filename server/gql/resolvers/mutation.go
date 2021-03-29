package resolvers

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/troydota/api.poll.komodohype.dev/mongo"
	"github.com/troydota/api.poll.komodohype.dev/redis"
	"github.com/troydota/api.poll.komodohype.dev/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type newInput struct {
	Title       string
	Options     []string
	CheckIP     *bool
	MultiAnswer *bool
	Expiry      *int32
}

func (*RootResolver) Vote(ctx context.Context, args struct {
	ID        string
	Selection []int32
}) (string, error) {
	id, err := primitive.ObjectIDFromHex(args.ID)
	if err != nil {
		return "", errMissingPoll
	}

	poll, err := fetchPoll(id, nil)
	if err != nil {
		return "", err
	}

	l := len(args.Selection)

	if l == 0 || l > len(poll.OptionsRaw) || l > 1 && !poll.MultiAnswer {
		return "INVALID_SELECTION", nil
	}

	if poll.Expiry != nil && poll.Expiry.Before(time.Now()) {
		return "EXPIRED", nil
	}

	_ip := ctx.Value(utils.Key("ip"))
	var ip string
	if _ip != nil {
		ip = _ip.(string)
	}

	if poll.CheckIP {
		if ip != "" {
			val, err := redis.Client.SAdd(redis.Ctx, fmt.Sprintf("poll:votes:%s:ips", poll.ID.Hex()), ip).Result()
			if err != nil && err != redis.ErrNil {
				log.Errorf("redis, err=%v", err)
				return "", errInternalServer
			}
			if val == 0 {
				return "ALREADY_VOTED", nil
			}
		} else {
			return "ALREADY_VOTED", nil
		}
	}

	_, err = mongo.Database.Collection("pollanswers").InsertOne(mongo.Ctx, mongo.PollAnswer{
		PollID: poll.ID,
		IP:     ip,
		Answer: args.Selection,
	})
	if err != nil {
		log.Errorf("mongo, err=%v", err)
	}

	pipe := redis.Client.Pipeline()

	selectionStr, err := json.MarshalToString(args.Selection)
	if err != nil {
		log.Errorf("json, err=%v", err)
	} else {
		event := fmt.Sprintf("events:poll:vote:%s", poll.ID.Hex())
		pipe.Publish(redis.Ctx, event, selectionStr)
	}

	for _, s := range args.Selection {
		pipe.HIncrBy(redis.Ctx, fmt.Sprintf("poll:votes:%s:options", poll.ID.Hex()), fmt.Sprint(s), 1)
	}

	_, err = pipe.Exec(redis.Ctx)
	if err != nil {
		log.Errorf("vote-pipe, err=%v", err)
	}

	return "SUCCESS", nil
}

type result struct {
	State string
	Poll  *pollResolver
}

func (*RootResolver) New(ctx context.Context, args struct {
	Poll newInput
}) (result, error) {
	if len(args.Poll.Title) > 64 || len(args.Poll.Title) == 0 {
		return result{"INVALID_TITLE", nil}, nil
	}

	if len(args.Poll.Options) < 2 || len(args.Poll.Options) > 15 {
		return result{"INVALID_OPTIONS", nil}, nil
	}
	for _, o := range args.Poll.Options {
		if len(o) > 64 || len(o) == 0 {
			return result{"INVALID_OPTIONS", nil}, nil
		}
	}

	var expiry int32
	if args.Poll.Expiry != nil {
		expiry = *args.Poll.Expiry
	}

	if expiry < 60 && expiry != 0 {
		return result{"INVALID_EXPIRY", nil}, nil
	}

	poll := &mongo.Poll{
		Title:      args.Poll.Title,
		OptionsRaw: args.Poll.Options,
	}

	if expiry > 0 {
		exp := time.Now().Add(time.Duration(expiry) * time.Second)
		poll.Expiry = &exp
	}

	if args.Poll.CheckIP != nil {
		poll.CheckIP = *args.Poll.CheckIP
	}
	if args.Poll.MultiAnswer != nil {
		poll.MultiAnswer = *args.Poll.MultiAnswer
	}

	res, err := mongo.Database.Collection("polls").InsertOne(mongo.Ctx, poll)
	if err != nil {
		log.Errorf("mongo, err=%v", err)
		return result{}, errInternalServer
	}
	poll.ID = res.InsertedID.(primitive.ObjectID)

	pollStr, err := json.MarshalToString(poll)
	if err == nil {
		if err = redis.Client.Set(redis.Ctx, fmt.Sprintf("cached:polls:%s", poll.ID.Hex()), pollStr, time.Hour*6).Err(); err != nil {
			log.Errorf("redis, err=%v", err)
		}
	} else {
		log.Errorf("redis, err=%v", err)
	}

	field := generateSelectedFieldMap(ctx)

	return result{"SUCCESS", &pollResolver{poll, field.children["poll"]}}, nil
}

type resultDraft struct {
	State string
	Poll  *draftResolver
}

func (*RootResolver) NewDraft(ctx context.Context, args struct {
	Poll newInput
}) (resultDraft, error) {
	if len(args.Poll.Title) > 64 || len(args.Poll.Title) == 0 {
		return resultDraft{"INVALID_TITLE", nil}, nil
	}

	if len(args.Poll.Options) < 2 || len(args.Poll.Options) > 15 {
		return resultDraft{"INVALID_OPTIONS", nil}, nil
	}
	for _, o := range args.Poll.Options {
		if len(o) > 64 || len(o) == 0 {
			return resultDraft{"INVALID_OPTIONS", nil}, nil
		}
	}

	var expiry int32
	if args.Poll.Expiry != nil {
		expiry = *args.Poll.Expiry
	}

	if expiry < 60 && expiry != 0 {
		return resultDraft{"INVALID_EXPIRY", nil}, nil
	}

	draft := &mongo.Draft{
		Title:   args.Poll.Title,
		Options: args.Poll.Options,
	}

	if expiry > 0 {
		draft.Expiry = &expiry
	}

	if args.Poll.CheckIP != nil {
		draft.CheckIP = *args.Poll.CheckIP
	}
	if args.Poll.MultiAnswer != nil {
		draft.MultiAnswer = *args.Poll.MultiAnswer
	}

	res, err := mongo.Database.Collection("drafts").InsertOne(mongo.Ctx, draft)
	if err != nil {
		log.Errorf("mongo, err=%v", err)
		return resultDraft{}, errInternalServer
	}
	draft.ID = res.InsertedID.(primitive.ObjectID)

	pollStr, err := json.MarshalToString(draft)
	if err == nil {
		if err = redis.Client.Set(redis.Ctx, fmt.Sprintf("cached:drafts:%s", draft.ID.Hex()), pollStr, time.Hour*6).Err(); err != nil {
			log.Errorf("redis, err=%v", err)
		}
	} else {
		log.Errorf("redis, err=%v", err)
	}
	return resultDraft{"SUCCESS", &draftResolver{draft}}, nil
}
