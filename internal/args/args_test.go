package args

import (
	"reflect"
	"testing"
)

func TestReorderMovesFlagsAheadOfPositionals(t *testing.T) {
	got := Reorder([]string{"session-id", "--limit", "3", "--prune-audio"}, map[string]bool{
		"--limit": true,
	})
	want := []string{"--limit", "3", "--prune-audio", "session-id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reorder() = %#v, want %#v", got, want)
	}
}

func TestReorderHandlesEqualsShortFlagsAndMissingValues(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "equals", input: []string{"id", "--limit=4"}, want: []string{"--limit=4", "id"}},
		{name: "short", input: []string{"id", "-x", "value"}, want: []string{"-x", "value", "id"}},
		{name: "dash", input: []string{"-", "id"}, want: []string{"-", "id"}},
		{name: "missing value", input: []string{"id", "--limit"}, want: []string{"--limit", "id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Reorder(test.input, map[string]bool{"--limit": true, "-x": true}); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Reorder() = %#v, want %#v", got, test.want)
			}
		})
	}
}
