// Package config loads ridgelined's runtime configuration from a JSON file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the daemon's runtime configuration.
type Config struct {
	// ListenAddr is the host:port the HTTP/WebSocket server binds to.
	ListenAddr string `json:"listenAddr"`
	// DBPath is the SQLite database file path.
	DBPath string `json:"dbPath"`
	// WebDir is the directory of built static web assets to serve. Empty
	// disables static serving (API only).
	WebDir string `json:"webDir"`
	// MQTT configures the upstream packet source.
	MQTT MQTT `json:"mqtt"`
	// NOTE: there is no admin token. The /api/admin/* endpoints are gated by the
	// account is_admin flag (session auth); the first account registered on a
	// fresh deployment becomes the protected owner/admin. A legacy "adminToken"
	// key in an old config.json is simply ignored.
	// ScrubArtifacts enables the periodic auto-removal of packet-corruption
	// artifacts (phantom node records whose public key arrived corrupted). Only
	// high-confidence artifacts are ever deleted. Defaults to true; set false to
	// disable the sweep.
	ScrubArtifacts bool `json:"scrubArtifacts"`
	// NodeRetentionDays is how long a node may go without ANY activity before the
	// daily retention sweep removes it. "Activity" means either an advert or a
	// relay: a node that adverted OR relayed a packet anywhere in the window is
	// kept. A removed node reappears the moment it transmits again, so this only
	// clears the genuinely-departed. Defaults to 7; set 0 to disable.
	NodeRetentionDays int `json:"nodeRetentionDays"`
	// ObserverRetentionMinutes is how long an observer may go without reporting
	// (no packet or status) before the retention sweep removes its row. Only the
	// observers row is deleted — the packets it reported are kept — and it
	// reappears the moment it publishes again, so this just tidies observers that
	// have gone silent. Defaults to 60; set 0 to disable.
	ObserverRetentionMinutes int `json:"observerRetentionMinutes"`
	// Email configures outbound transactional mail (verification + notifications).
	// When Host is empty, email is disabled and those features degrade gracefully.
	Email Email `json:"email"`
	// Environment names this instance's role, surfaced on /api/health. Set it to
	// "dev" (or "staging") on a non-production box to make the UI show a prominent
	// "not the live site" banner. Empty (the default) means a normal instance with
	// no banner — so production instances get nothing unless they opt in.
	Environment string `json:"environment"`
}

// Email configures the outbound SMTP relay for transactional mail. For Brevo:
// Host smtp-relay.brevo.com, Port 587, Username the Brevo SMTP login (e.g.
// xxxxxxx@smtp-brevo.com), Password a Brevo SMTP key. From must be an address on
// a domain authenticated at the relay (SPF/DKIM), and BaseURL is the public site
// origin used to build verification links.
type Email struct {
	Host     string `json:"host"`     // SMTP submission host; empty disables email
	Port     int    `json:"port"`     // 587 (STARTTLS) or 465 (implicit TLS)
	Username string `json:"username"` // SMTP auth user (Brevo SMTP login)
	Password string `json:"password"` // SMTP auth password / API key
	From     string `json:"from"`     // envelope + header From, e.g. noreply@example.com
	FromName string `json:"fromName"` // display name, e.g. "Ridgeline"
	BaseURL  string `json:"baseURL"`  // public origin, e.g. https://ridgeline.example.com
}

// Enabled reports whether outbound email is fully configured. Requiring the
// password too means the config block can be pre-filled with everything except
// the API key, and email stays safely disabled (registration auto-verifies)
// until the key is added — no accounts get stranded on a keyless instance.
func (e Email) Enabled() bool { return e.Host != "" && e.From != "" && e.Password != "" }

// MQTT configures the connection to a MeshCore observer broker.
type MQTT struct {
	Broker   string   `json:"broker"`   // e.g. tcp://host:1883
	ClientID string   `json:"clientID"` // MQTT client id (must be unique on the broker)
	Username string   `json:"username"`
	Password string   `json:"password"`
	Topics   []string `json:"topics"` // subscriptions, e.g. meshcore/+/+/packets
}

// Default returns a Config populated with sensible defaults for local
// development against the dev MeshCore broker.
func Default() Config {
	return Config{
		ListenAddr:               ":8080",
		DBPath:                   "ridgeline.db",
		WebDir:                   "web/build",
		ScrubArtifacts:           true,
		NodeRetentionDays:        7,
		ObserverRetentionMinutes: 60,
		Email: Email{
			Port:     587,
			FromName: "Ridgeline",
			// No default BaseURL: each instance must set its own public origin.
			// A hardcoded default here would silently send an instance's links to
			// that origin whenever the field is omitted. mail.New warns if this is
			// empty while email is enabled.
		},
		MQTT: MQTT{
			Broker:   "tcp://localhost:1883",
			ClientID: "ridgelined",
			Topics:   []string{"meshcore/+/+/packets"},
		},
	}
}

// Load reads and parses the config file at path, applying defaults for any
// fields left unset.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	// Decode over the defaults so omitted fields keep their default values.
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "ridgeline.db"
	}
	if cfg.MQTT.ClientID == "" {
		cfg.MQTT.ClientID = "ridgelined"
	}
	if len(cfg.MQTT.Topics) == 0 {
		cfg.MQTT.Topics = []string{"meshcore/+/+/packets"}
	}
	return cfg, nil
}
