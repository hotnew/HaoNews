package live

import (
	"context"
	"reflect"
	"testing"
)

func TestIsSessionExitCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "exit", input: "/exit", want: true},
		{name: "quit", input: "/quit", want: true},
		{name: "trimmed", input: "  /EXIT  ", want: true},
		{name: "message", input: "hello", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isSessionExitCommand(tt.input); got != tt.want {
				t.Fatalf("isSessionExitCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleInputLineExitCommandDoesNotPublish(t *testing.T) {
	t.Parallel()

	s := &session{}
	exitRequested, err := s.handleInputLine(context.Background(), "/exit")
	if err != nil {
		t.Fatalf("handleInputLine returned error: %v", err)
	}
	if !exitRequested {
		t.Fatalf("handleInputLine should request exit for /exit")
	}
}

func TestSessionStartCleanupRunsInReverseOrder(t *testing.T) {
	t.Parallel()

	var got []string
	cleanup := &sessionStartCleanup{}
	cleanup.add(func() { got = append(got, "host") })
	cleanup.add(func() { got = append(got, "dht") })
	cleanup.add(func() { got = append(got, "topic") })
	cleanup.run()

	want := []string{"topic", "dht", "host"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup order = %#v, want %#v", got, want)
	}
}
