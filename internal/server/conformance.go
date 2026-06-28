package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
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
	if err := expectSessionState(server.URL, created.ID, SessionIdle); err != nil {
		return err
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
	body = bytes.NewBufferString(`{"command":"HELP"}`)
	resp, err = http.Post(server.URL+"/sessions/"+created.ID+"/run", "application/json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("run status=%d", resp.StatusCode)
	}
	var accepted struct {
		Accepted bool   `json:"accepted"`
		Output   string `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		return err
	}
	if !accepted.Accepted {
		return fmt.Errorf("run non accettato")
	}
	output := accepted.Output
	for i := 0; i < 20 && !strings.Contains(output, "DIR"); i++ {
		resp, err = http.Get(server.URL + "/sessions/" + created.ID + "/output")
		if err != nil {
			return err
		}
		var drained struct {
			Output string `json:"output"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&drained); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		output += drained.Output
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(output, "DIR") {
		return fmt.Errorf("output run async inatteso: %q", output)
	}
	return nil
}

func expectSessionState(baseURL string, id string, state SessionState) error {
	resp, err := http.Get(baseURL + "/sessions/" + id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET session status=%d", resp.StatusCode)
	}
	var result struct {
		State SessionState `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.State != state {
		return fmt.Errorf("session state=%q, want %q", result.State, state)
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
