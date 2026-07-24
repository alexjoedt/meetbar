package config

import "testing"

func TestExampleConfigParses(t *testing.T) {
	cfg, err := Load("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%#v", cfg)
}
