package resolvers

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	errPollNotFound = fmt.Errorf("poll not found")
)

func (r *RootResolver) Watch(ctx context.Context, args struct{ ID string }) (<-chan *pollResolver, error) {
	field := generateSelectedFieldMap(ctx)

	id, err := primitive.ObjectIDFromHex(args.ID)
	if err != nil {
		return nil, errPollNotFound
	}

	poll, err := fetchPoll(id, field)
	if err != nil {
		return nil, err
	}
	if poll == nil {
		return nil, errPollNotFound
	}

	resolver := &pollResolver{}
	rChan := make(chan *pollResolver, 1)

	fetchVotes := false
	if v, ok := field.children["options"]; ok {
		if _, ok := v.children["votes"]; ok {
			fetchVotes = true
		}
	}

	resolver.poll = poll

	if fetchVotes {
		vote := make(chan PollVote, 100)

		err = r.subscribe(fmt.Sprintf("events:poll:vote:%s", poll.ID.Hex()), vote)
		if err != nil {
			log.Errorf("redis, err=%v", err)
			return nil, errInternalServer
		}
		go func() {
			for {
				select {
				case <-ctx.Done():
					err = r.unsubscribe(fmt.Sprintf("events:poll:vote:%s", poll.ID.Hex()), vote)
					if err != nil {
						log.Errorf("redis, err=%v", err)
					}
					return
				case v := <-vote:
					opts := *poll.Options
					for _, s := range v {
						opts[s].Votes++
					}
					poll.Options = &opts
					rChan <- resolver
				}
			}
		}()
	}
	defer func() {
		rChan <- resolver
		if !fetchVotes {
			close(rChan)
		}
	}()
	return rChan, nil
}
