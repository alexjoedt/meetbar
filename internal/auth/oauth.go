package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
	oauth2api "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

// tokenInfoURL is a var (not const) so tests can point it at a local server.
var tokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

// CalendarReadonlyScope is required for CalendarList + Events read.
const CalendarReadonlyScope = gcal.CalendarReadonlyScope

var Scopes = []string{
	CalendarReadonlyScope,
	"https://www.googleapis.com/auth/userinfo.email",
}

type Manager struct {
	mu            sync.Mutex
	creds         Credentials
	tokenPath     string
	config        *oauth2.Config
	token         *oauth2.Token
	email         string
	ts            oauth2.TokenSource
	loginInFlight bool
}

func NewManager(creds Credentials, tokenPath string) (*Manager, error) {
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       Scopes,
		// RedirectURL set per login on ephemeral port.
	}
	m := &Manager{creds: creds, tokenPath: tokenPath, config: cfg}
	tok, email, err := LoadToken(tokenPath)
	if err == nil && tok != nil {
		m.token = tok
		m.email = email
		m.ts = cfg.TokenSource(context.Background(), tok)
	}
	return m, nil
}

func (m *Manager) Status() (auth string, email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == nil {
		return "disconnected", ""
	}
	// Connected if we have a refresh token or a still-valid access token.
	if m.token.RefreshToken == "" && !m.token.Valid() {
		return "disconnected", ""
	}
	return "connected", m.email
}

func (m *Manager) HTTPClient(ctx context.Context) (*http.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	if m.ts == nil {
		m.ts = m.config.TokenSource(ctx, m.token)
	}
	return oauth2.NewClient(ctx, &savingTokenSource{inner: m.ts, m: m}), nil
}

type savingTokenSource struct {
	inner oauth2.TokenSource
	m     *Manager
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.inner.Token()
	if err != nil {
		return nil, err
	}
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if s.m.token == nil || tok.AccessToken != s.m.token.AccessToken {
		s.m.token = tok
		_ = SaveToken(s.m.tokenPath, tok, s.m.email)
	}
	return tok, nil
}

func (m *Manager) Login(ctx context.Context) (string, error) {
	m.mu.Lock()
	if m.loginInFlight {
		m.mu.Unlock()
		return "", fmt.Errorf("login already in progress")
	}
	m.loginInFlight = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.loginInFlight = false
		m.mu.Unlock()
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("loopback listen: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	cfg := *m.config
	cfg.RedirectURL = redirectURL

	verifier := oauth2.GenerateVerifier()
	state := fmt.Sprintf("%d", time.Now().UnixNano())
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
		oauth2.S256ChallengeOption(verifier),
	)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("error") != "" {
			errCh <- fmt.Errorf("oauth error: %s", r.URL.Query().Get("error"))
			fmt.Fprint(w, "<html><body><h1>Login failed</h1><p>You can close this window.</p></body></html>")
			return
		}
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("invalid oauth state")
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("missing code")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "<html><body><h1>Meetbar connected</h1><p>You can close this window.</p></body></html>")
		codeCh <- code
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Open this URL in your browser:\n%s\n", authURL)
	} else {
		fmt.Println("Opening browser for Google login…")
	}

	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	var code string
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case code = <-codeCh:
	case <-timeout.C:
		return "", fmt.Errorf("login timed out")
	}

	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}

	if err := requireCalendarScope(ctx, tok); err != nil {
		return "", err
	}

	email, err := fetchEmail(ctx, &cfg, tok)
	if err != nil {
		return "", err
	}
	if err := SaveToken(m.tokenPath, tok, email); err != nil {
		return "", err
	}

	m.mu.Lock()
	m.token = tok
	m.email = email
	m.config.RedirectURL = ""
	m.ts = cfg.TokenSource(context.Background(), tok)
	m.mu.Unlock()

	return email, nil
}

func (m *Manager) Logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = nil
	m.email = ""
	m.ts = nil
	return ClearToken(m.tokenPath)
}

func fetchEmail(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (string, error) {
	client := cfg.Client(ctx, tok)
	svc, err := oauth2api.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", err
	}
	ui, err := svc.Userinfo.Get().Do()
	if err != nil {
		return "", fmt.Errorf("userinfo: %w", err)
	}
	if ui.Email == "" {
		return "", fmt.Errorf("userinfo: empty email")
	}
	return ui.Email, nil
}

func requireCalendarScope(ctx context.Context, tok *oauth2.Token) error {
	scopes := tokenScopes(tok)
	if hasScope(scopes, CalendarReadonlyScope) || hasScope(scopes, gcal.CalendarScope) {
		return nil
	}
	// Token response sometimes omits scope; ask tokeninfo.
	info, err := fetchTokenInfo(ctx, tok.AccessToken)
	if err == nil {
		scopes = info
		if hasScope(scopes, CalendarReadonlyScope) || hasScope(scopes, gcal.CalendarScope) {
			return nil
		}
	}
	return fmt.Errorf("calendar permission missing (granted: %s). On the Google consent screen enable “See and download any calendar…”, then run: meetbarctl logout && meetbarctl login", strings.Join(scopes, " "))
}

func tokenScopes(tok *oauth2.Token) []string {
	if tok == nil {
		return nil
	}
	if s, ok := tok.Extra("scope").(string); ok && s != "" {
		return strings.Fields(s)
	}
	return nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func fetchTokenInfo(ctx context.Context, accessToken string) ([]string, error) {
	u := tokenInfoURL + "?access_token=" + url.QueryEscape(accessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tokeninfo %s", res.Status)
	}
	var info struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("tokeninfo: decode: %w", err)
	}
	if info.Scope == "" {
		return nil, fmt.Errorf("tokeninfo: no scope")
	}
	return strings.Fields(info.Scope), nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}
