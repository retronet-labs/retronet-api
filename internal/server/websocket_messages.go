package server

type socketMessage struct {
	Type     string       `json:"type"`
	Data     string       `json:"data,omitempty"`
	Command  string       `json:"command,omitempty"`
	Snapshot any          `json:"snapshot,omitempty"`
	Accepted bool         `json:"accepted,omitempty"`
	Closed   bool         `json:"closed,omitempty"`
	State    SessionState `json:"state,omitempty"`
	Error    string       `json:"error,omitempty"`
}
