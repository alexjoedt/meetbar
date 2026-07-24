package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Embedded desktop OAuth client. Override at build time:
//
//	go build -ldflags "-X github.com/alex/meetbar/internal/auth.embeddedClientID=... -X github.com/alex/meetbar/internal/auth.embeddedClientSecret=..."
//
// Or set MEETBAR_CLIENT_ID / MEETBAR_CLIENT_SECRET, or place credentials.json
// next to config / via MEETBAR_CREDENTIALS.
var (
	embeddedClientID     = ""
	embeddedClientSecret = ""
)

type Credentials struct {
	ClientID     string
	ClientSecret string
}

type credentialsFile struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
	Web *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"web"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func ResolveCredentials(paths ...string) (Credentials, error) {
	if id := os.Getenv("MEETBAR_CLIENT_ID"); id != "" {
		return Credentials{
			ClientID:     id,
			ClientSecret: os.Getenv("MEETBAR_CLIENT_SECRET"),
		}, nil
	}
	if p := os.Getenv("MEETBAR_CREDENTIALS"); p != "" {
		c, err := loadCredentialsFile(p)
		if err != nil {
			return Credentials{}, err
		}
		return c, nil
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		c, err := loadCredentialsFile(p)
		if err == nil {
			return c, nil
		}
		if !os.IsNotExist(err) {
			return Credentials{}, err
		}
	}
	if strings.TrimSpace(embeddedClientID) != "" {
		return Credentials{
			ClientID:     embeddedClientID,
			ClientSecret: embeddedClientSecret,
		}, nil
	}
	return Credentials{}, fmt.Errorf("no OAuth credentials: set MEETBAR_CLIENT_ID, MEETBAR_CREDENTIALS, place credentials.json in config dir, or embed client id at build time")
}

func loadCredentialsFile(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}
	var f credentialsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Credentials{}, fmt.Errorf("credentials %s: %w", path, err)
	}
	if f.Installed != nil && f.Installed.ClientID != "" {
		return Credentials{ClientID: f.Installed.ClientID, ClientSecret: f.Installed.ClientSecret}, nil
	}
	if f.Web != nil && f.Web.ClientID != "" {
		return Credentials{ClientID: f.Web.ClientID, ClientSecret: f.Web.ClientSecret}, nil
	}
	if f.ClientID != "" {
		return Credentials{ClientID: f.ClientID, ClientSecret: f.ClientSecret}, nil
	}
	return Credentials{}, fmt.Errorf("credentials %s: missing client_id", path)
}
