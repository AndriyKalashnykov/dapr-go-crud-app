package todos

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Todo struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	CreatedAt time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time     `bson:"updated_at" json:"updated_at"`
	Id        string        `form:"id" json:"id" bson:"todoId"`
	Text      string        `form:"text" json:"text" binding:"required" bson:"text"`
	Done      string        `form:"done" json:"done" bson:"done"`
	Deleted   string        `form:"deleted" json:"deleted" bson:"deleted"`
}
