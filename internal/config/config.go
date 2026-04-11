package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	// Server
	Host   string `json:"host"`
	Port   string `json:"port"`
	Domain     string `json:"domain"`
	AIMetadata bool   `json:"ai_metadata"`

	// Author
	AuthorName    string `json:"author_name"`
	AuthorBio     string `json:"author_bio"`
	AuthorAvatar  string `json:"author_avatar"`

	// Content
	ContentDir string `json:"content_dir"`

	// Auth — owner keypair (base64 encoded)
	OwnerPublicKey  string `json:"owner_public_key"`
	OwnerPrivateKey string `json:"owner_private_key"`

	// Edit token
	EditToken string `json:"edit_token"`

	// Ed25519 signing keypair (base64 private key, hex public key)
	SigningPrivateKey string `json:"signing_private_key"`
	SigningPublicKey  string `json:"signing_public_key"`

	// Agent token — trusted agents can write skills/personas
	AgentToken string `json:"agent_token"`

	// Session code rotation interval in hours (default 24)
	SessionRotateHours int `json:"session_rotate_hours"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:       "0.0.0.0",
		Port:       "8080",
		Domain:     "localhost:8080",
		AuthorName: "Anonymous",
		AuthorBio:  "A human with something to say.",
		ContentDir: "./content",
		EditToken:           os.Getenv("EDIT_TOKEN"),
		SessionRotateHours:  24,
	}

	// Override from env vars (12-factor)
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("DOMAIN"); v != "" {
		cfg.Domain = v
	}
	if v := os.Getenv("AI_METADATA"); v == "true" {
		cfg.AIMetadata = true
	}
	if v := os.Getenv("AUTHOR_NAME"); v != "" {
		cfg.AuthorName = v
	}
	if v := os.Getenv("AUTHOR_BIO"); v != "" {
		cfg.AuthorBio = v
	}
	if v := os.Getenv("CONTENT_DIR"); v != "" {
		cfg.ContentDir = v
	}
	if v := os.Getenv("SIGNING_PRIVATE_KEY"); v != "" {
		cfg.SigningPrivateKey = v
	}
	if v := os.Getenv("SESSION_ROTATE_HOURS"); v != "" {
		var h int
		if _, err := fmt.Sscanf(v, "%d", &h); err == nil && h > 0 {
			cfg.SessionRotateHours = h
		}
	}
	if v := os.Getenv("AGENT_TOKEN"); v != "" {
		cfg.AgentToken = v
	}
	if v := os.Getenv("SIGNING_PUBLIC_KEY"); v != "" {
		cfg.SigningPublicKey = v
	}

	// Load from config.json if present
	cfgPath := "config.json"
	if _, err := os.Stat(cfgPath); err == nil {
		data, err := os.ReadFile(cfgPath)
		if err == nil {
			_ = json.Unmarshal(data, cfg)
		}
	}

	// Resolve content dir
	abs, err := filepath.Abs(cfg.ContentDir)
	if err == nil {
		cfg.ContentDir = abs
	}

	return cfg, nil
}
