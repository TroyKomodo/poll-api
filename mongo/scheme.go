package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Poll struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Title       string             `json:"title" bson:"title"`
	OptionsRaw  []string           `json:"options" bson:"options"`
	CheckIP     bool               `json:"check_ip" bson:"check_ip"`
	MultiAnswer bool               `json:"multi_answer" bson:"multi_answer"`
	Expiry      *time.Time         `json:"expiry" bson:"expiry"`

	Options *[]PollOption `json:"-" bson:"-"`
}

type Draft struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Title       string             `json:"title" bson:"title"`
	Options     []string           `json:"options" bson:"options"`
	CheckIP     bool               `json:"check_ip" bson:"check_ip"`
	MultiAnswer bool               `json:"multi_answer" bson:"multi_answer"`
	Expiry      *int32             `json:"expiry" bson:"expiry"`
}

type PollOption struct {
	Title string `json:"title"`
	Votes int32  `json:"votes"`
}

type PollAnswer struct {
	ID     primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PollID primitive.ObjectID `json:"poll_id" bson:"poll_id"`
	IP     string             `json:"ip" bson:"ip"`
	Answer []int32            `json:"answer" bson:"answer"`
}
