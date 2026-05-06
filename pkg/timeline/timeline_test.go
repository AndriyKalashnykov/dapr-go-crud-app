package timeline

import (
	"reflect"
	"testing"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
)

func TestHandle_AppendsBranchSpecificEntry(t *testing.T) {
	tests := []struct {
		name string
		todo todos.Todo
		want string
	}{
		{"deleted wins", todos.Todo{Text: "X", Deleted: "true", Done: "true"}, "Todo deleted: X"},
		{"done", todos.Todo{Text: "Y", Done: "true"}, "Todo marked as done: Y"},
		{"new (default)", todos.Todo{Text: "Z"}, "New todo created: Z"},
		{"empty done/deleted falls through", todos.Todo{Text: "Q", Done: "", Deleted: ""}, "New todo created: Q"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tl := New()
			tl.Handle(tc.todo)
			if got := tl.Timeline(); !reflect.DeepEqual(got, []string{tc.want}) {
				t.Errorf("Timeline() = %v, want [%q]", got, tc.want)
			}
		})
	}
}

func TestHandle_PreservesInsertionOrder(t *testing.T) {
	tl := New()
	for _, td := range []todos.Todo{
		{Text: "first"},
		{Text: "second", Done: "true"},
		{Text: "third", Deleted: "true"},
	} {
		tl.Handle(td)
	}

	want := []string{
		"New todo created: first",
		"Todo marked as done: second",
		"Todo deleted: third",
	}
	if got := tl.Timeline(); !reflect.DeepEqual(got, want) {
		t.Errorf("Timeline() = %v, want %v", got, want)
	}
}

func TestNew_StartsEmpty(t *testing.T) {
	if got := New().Timeline(); len(got) != 0 {
		t.Errorf("New().Timeline() = %v, want empty slice", got)
	}
}
