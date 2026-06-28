package main

import "testing"

func TestParseOrigins(t *testing.T) {
	got := parseOrigins(" http://127.0.0.1:18081, ,http://localhost:18081 ")
	want := []string{"http://127.0.0.1:18081", "http://localhost:18081"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origin[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
