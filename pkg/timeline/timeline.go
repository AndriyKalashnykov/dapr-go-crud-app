package timeline

import "github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"

type Timeline interface {
	Timeline() []string
	Handle(t todos.Todo)
}

type timelineApi struct {
	timeline []string
}

func New() Timeline {
	return &timelineApi{
		timeline: make([]string, 0),
	}
}

func (a *timelineApi) Timeline() []string {
	return a.timeline
}

func (a *timelineApi) Handle(t todos.Todo) {
	var ev string
	switch {
	case t.Deleted == "true":
		ev = "Todo deleted: " + t.Text
	case t.Done == "true":
		ev = "Todo marked as done: " + t.Text
	default:
		ev = "New todo created: " + t.Text
	}
	a.timeline = append(a.timeline, ev)
}
