package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

type storedToken struct {
	Token *oauth2.Token `json:"token"`
	Email string        `json:"email"`
}

func LoadToken(path string) (*oauth2.Token, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var st storedToken
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, "", fmt.Errorf("token file: %w", err)
	}
	if st.Token == nil {
		return nil, "", fmt.Errorf("token file empty")
	}
	return st.Token, st.Email, nil
}

func SaveToken(path string, tok *oauth2.Token, email string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	st := storedToken{Token: tok, Email: email}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ClearToken(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
