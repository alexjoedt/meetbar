package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCredentials(t *testing.T) {
	dir := t.TempDir()

	installedPath := filepath.Join(dir, "installed.json")
	writeJSON(t, installedPath, `{"installed":{"client_id":"installed-id","client_secret":"installed-secret"}}`)

	webPath := filepath.Join(dir, "web.json")
	writeJSON(t, webPath, `{"web":{"client_id":"web-id","client_secret":"web-secret"}}`)

	flatPath := filepath.Join(dir, "flat.json")
	writeJSON(t, flatPath, `{"client_id":"flat-id","client_secret":"flat-secret"}`)

	missingClientIDPath := filepath.Join(dir, "bad.json")
	writeJSON(t, missingClientIDPath, `{"foo":"bar"}`)

	missingPath := filepath.Join(dir, "does-not-exist.json")

	tests := []struct {
		name            string
		envClientID     string
		envClientSecret string
		envCredsPath    string
		paths           []string
		embeddedID      string
		embeddedSecret  string
		wantID          string
		wantSecret      string
		wantErr         bool
	}{
		{
			name:            "env client id wins over everything",
			envClientID:     "env-id",
			envClientSecret: "env-secret",
			paths:           []string{installedPath},
			embeddedID:      "embedded-id",
			wantID:          "env-id",
			wantSecret:      "env-secret",
		},
		{
			name:         "env credentials path wins over path list",
			envCredsPath: webPath,
			paths:        []string{installedPath},
			wantID:       "web-id",
			wantSecret:   "web-secret",
		},
		{
			name:       "installed key in credentials file",
			paths:      []string{installedPath},
			wantID:     "installed-id",
			wantSecret: "installed-secret",
		},
		{
			name:       "flat client_id fallback",
			paths:      []string{flatPath},
			wantID:     "flat-id",
			wantSecret: "flat-secret",
		},
		{
			name:       "skips missing paths and uses next match",
			paths:      []string{missingPath, installedPath},
			wantID:     "installed-id",
			wantSecret: "installed-secret",
		},
		{
			name:    "file exists but missing client_id errors",
			paths:   []string{missingClientIDPath},
			wantErr: true,
		},
		{
			name:           "falls back to embedded credentials",
			paths:          []string{missingPath},
			embeddedID:     "embedded-id",
			embeddedSecret: "embedded-secret",
			wantID:         "embedded-id",
			wantSecret:     "embedded-secret",
		},
		{
			name:    "errors when nothing is configured",
			paths:   []string{missingPath},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MEETBAR_CLIENT_ID", tt.envClientID)
			t.Setenv("MEETBAR_CLIENT_SECRET", tt.envClientSecret)
			t.Setenv("MEETBAR_CREDENTIALS", tt.envCredsPath)

			origID, origSecret := embeddedClientID, embeddedClientSecret
			embeddedClientID = tt.embeddedID
			embeddedClientSecret = tt.embeddedSecret
			t.Cleanup(func() {
				embeddedClientID = origID
				embeddedClientSecret = origSecret
			})

			got, err := ResolveCredentials(tt.paths...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveCredentials() error = nil, want error (got %#v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveCredentials() unexpected error: %v", err)
			}
			if got.ClientID != tt.wantID || got.ClientSecret != tt.wantSecret {
				t.Fatalf("ResolveCredentials() = %#v, want {%s %s}", got, tt.wantID, tt.wantSecret)
			}
		})
	}
}
