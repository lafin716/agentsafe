package session

import "time"

type Metadata struct {
	Feature   string    `json:"feature"`
	CreatedAt time.Time `json:"createdAt"`
	AgentPath string    `json:"agentPath"`
	Commands  []string  `json:"commands,omitempty"`
}
