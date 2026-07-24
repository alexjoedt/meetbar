package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"golang.org/x/oauth2"
)

func TestHasScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   string
		expect bool
	}{
		{name: "present", scopes: []string{"a", "b", "c"}, want: "b", expect: true},
		{name: "absent", scopes: []string{"a", "b"}, want: "z", expect: false},
		{name: "empty list", scopes: nil, want: "a", expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasScope(tt.scopes, tt.want); got != tt.expect {
				t.Fatalf("hasScope(%v, %q) = %v, want %v", tt.scopes, tt.want, got, tt.expect)
			}
		})
	}
}

func withScopeExtra(scope string) *oauth2.Token {
	return (&oauth2.Token{}).WithExtra(map[string]any{"scope": scope})
}

func TestTokenScopes(t *testing.T) {
	tests := []struct {
		name string
		tok  *oauth2.Token
		want []string
	}{
		{name: "nil token", tok: nil, want: nil},
		{name: "no extra scope", tok: &oauth2.Token{}, want: nil},
		{name: "scope present", tok: withScopeExtra("a b c"), want: []string{"a", "b", "c"}},
		{name: "empty scope string", tok: withScopeExtra(""), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenScopes(tt.tok)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("tokenScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchTokenInfo(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantScopes []string
		wantErr    bool
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"scope": "a b c"})
			},
			wantScopes: []string{"a", "b", "c"},
		},
		{
			name: "non-200 status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			wantErr: true,
		},
		{
			name: "missing scope field",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"foo": "bar"})
			},
			wantErr: true,
		},
		{
			name: "invalid json body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("not json"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			orig := tokenInfoURL
			tokenInfoURL = srv.URL
			t.Cleanup(func() { tokenInfoURL = orig })

			got, err := fetchTokenInfo(t.Context(), "fake-token")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("fetchTokenInfo() error = nil, want error (got %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchTokenInfo() unexpected error: %v", err)
			}
			if !slices.Equal(got, tt.wantScopes) {
				t.Fatalf("fetchTokenInfo() = %v, want %v", got, tt.wantScopes)
			}
		})
	}
}
