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
	resp, err = http.Get(ts.URL + "/sessions/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		State SessionState `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if state.State != SessionIdle {
		t.Fatalf("state=%q", state.State)
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
	if result.State != SessionIdle {
		t.Fatalf("result state=%q", result.State)
	}
}

func TestAsyncRunInputAndOutput(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createTestSession(t, ts.URL)
	body := bytes.NewBufferString(`{"command":"HELP"}`)
	resp, err := http.Post(ts.URL+"/sessions/"+sessionID+"/run", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run status=%d", resp.StatusCode)
	}
	var accepted asyncResult
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if !accepted.Accepted {
		t.Fatal("run non accettato")
	}

	output := accepted.Output + pollOutput(t, ts.URL, sessionID, "DIR")
	if !strings.Contains(output, "DIR") || !strings.Contains(output, "A>") {
		t.Fatalf("output async=%q", output)
	}

	input := bytes.NewBufferString(`{"data":"HELP\r"}`)
	resp, err = http.Post(ts.URL+"/sessions/"+sessionID+"/input", "application/json", input)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("input status=%d", resp.StatusCode)
	}
	var inputResult commandResult
	if err := json.NewDecoder(resp.Body).Decode(&inputResult); err != nil {
		t.Fatal(err)
	}
	output = inputResult.Output + pollOutput(t, ts.URL, sessionID, "DIR")
	if !strings.Contains(output, "HELP") || !strings.Contains(output, "DIR") {
		t.Fatalf("output input=%q", output)
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

func TestCORSAllowedOrigin(t *testing.T) {
	app := New(Config{AllowedOrigins: []string{"http://127.0.0.1:18081"}})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:18081")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:18081" {
		t.Fatalf("allow-origin=%q", got)
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	app := New(Config{AllowedOrigins: []string{"http://127.0.0.1:18081"}})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin=%q", got)
	}
}

func TestRunConformance(t *testing.T) {
	if err := RunConformance(t.Context(), Config{Version: "test"}); err != nil {
		t.Fatal(err)
	}
}

func createTestSession(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/sessions", "application/json", nil)
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
	return created.ID
}

func pollOutput(t *testing.T, baseURL string, sessionID string, want string) string {
	t.Helper()
	var all strings.Builder
	for i := 0; i < 20; i++ {
		resp, err := http.Get(baseURL + "/sessions/" + sessionID + "/output")
		if err != nil {
			t.Fatal(err)
		}
		var result commandResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		all.WriteString(result.Output)
		if strings.Contains(all.String(), want) && result.State != SessionRunning {
			return all.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	return all.String()
}
