package entergrain

import (
	"reflect"
	"testing"
)

func TestParseEnviron(t *testing.T) {
	t.Parallel()

	got := parseEnviron([]byte("USER=alice\x00HOME=/home/alice\x00"))
	want := []string{"USER=alice", "HOME=/home/alice"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnviron() = %#v, want %#v", got, want)
	}
}

func TestParseEnvironIgnoresEmptyEntries(t *testing.T) {
	t.Parallel()

	got := parseEnviron([]byte("\x00USER=alice\x00\x00HOME=/home/alice"))
	want := []string{"USER=alice", "HOME=/home/alice"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnviron() = %#v, want %#v", got, want)
	}
}
