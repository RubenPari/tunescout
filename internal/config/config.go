// Package config gestisce percorsi e impostazioni di tunescout.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultModel  = "claude-opus-5-5"
	DefaultEffort = "medium"
	RedirectURL   = "http://127.0.0.1:8888/callback"
)

type Config struct {
	SpotifyClientID string `json:"spotify_client_id"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
}

// Dir restituisce ~/.config/tunescout, creandola se necessario.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "tunescout")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func path(name string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}

func TokenPath() (string, error) { return path("token.json") }
func DBPath() (string, error)    { return path("tunescout.db") }

// Load legge config.json e applica gli override da variabili d'ambiente.
func Load() (*Config, error) {
	c := &Config{}
	p, err := path("config.json")
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("config.json non valido: %w", err)
		}
	}
	if v := os.Getenv("SPOTIFY_CLIENT_ID"); v != "" {
		c.SpotifyClientID = v
	}
	if v := os.Getenv("TUNESCOUT_MODEL"); v != "" {
		c.Model = v
	}
	return c, nil
}

// WithDefaults restituisce una copia con modello ed effort di default applicati
// (non salvati su disco, così un default aggiornato vale anche per config esistenti).
func (c Config) WithDefaults() *Config {
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.Effort == "" {
		c.Effort = DefaultEffort
	}
	return &c
}

func (c *Config) Save() error {
	p, err := path("config.json")
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
