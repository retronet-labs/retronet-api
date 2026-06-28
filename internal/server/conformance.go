package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

func RunConformance(ctx context.Context, config Config) error {
	_ = ctx
	app := New(config)
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	if err := expectGET(server.URL + "/health"); err != nil {
		return err
	}
	resp, err := http.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("POST /sessions status=%d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return err
	}
	if created.ID == "" {
		return fmt.Errorf("session id vuoto")
	}
	body := bytes.NewBufferString(`{"command":"HELP"}`)
	resp, err = http.Post(server.URL+"/sessions/"+created.ID+"/command", "application/json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("command status=%d", resp.StatusCode)
	}
	var result struct {
		Output string `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !strings.Contains(result.Output, "DIR") {
		return fmt.Errorf("output HELP inatteso: %q", result.Output)
	}
	return nil
}

func expectGET(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s status=%d", url, resp.StatusCode)
	}
	return nil
}
