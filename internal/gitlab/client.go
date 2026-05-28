package gitlab

import "github.com/agentsafe/agentsafe/internal/config"

type Client struct {
	BaseURL string
	Token   string
}

func New(cfg config.GitLabConfig, token string) Client {
	return Client{BaseURL: cfg.BaseURL, Token: token}
}
