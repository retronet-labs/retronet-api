package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthVersionAndCommand(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("id vuoto")
	}

	body := bytes.NewBufferString(`{"command":"HELP"}`)
	resp, err = http.Post(ts.URL+"/sessions/"+created.ID+"/command", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("command status=%d", resp.StatusCode)
	}
	var result commandResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "DIR") || !strings.Contains(result.Output, "A>") {
		t.Fatalf("output=%q", result.Output)
	}
}

func TestSessionLimit(t *testing.T) {
	app := New(Config{MaxSessions: 1})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = http.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestRunConformance(t *testing.T) {
	if err := RunConformance(t.Context(), Config{Version: "test"}); err != nil {
		t.Fatal(err)
	}
}
