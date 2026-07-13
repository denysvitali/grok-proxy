package config

import "testing"

func TestResolveModel(t *testing.T) {
	cfg := Config{Proxy: ProxyConfig{
		DefaultModel: "grok-4.5",
		ModelMap:     map[string]string{"claude-special": "grok-composer-2.5-fast"},
	}}
	tests := []struct {
		requested string
		want      string
	}{
		{"grok-4.5", "grok-4.5"},
		{"claude-special", "grok-composer-2.5-fast"},
		{"claude-sonnet", "grok-4.5"},
		{"gpt-codex", "grok-4.5"},
	}
	for _, test := range tests {
		if got := cfg.ResolveModel(test.requested); got != test.want {
			t.Errorf("ResolveModel(%q) = %q, want %q", test.requested, got, test.want)
		}
	}
}
