package provider

import (
	"testing"
)

// An environment may declare the provider with no credentials while managing
// zero GetStream resources (dev → qa → prod rollout). Configure must not error;
// the error must surface only when a resource asks for the client.
func TestProviderData_RequireClientDefersMissingCredentials(t *testing.T) {
	pd := &providerData{unconfiguredReason: "Missing GetStream.io api_key"}
	var got []string
	c := pd.requireClient(func(summary, detail string) { got = append(got, summary, detail) })
	if c != nil {
		t.Fatalf("expected nil client for an unconfigured provider, got %v", c)
	}
	if len(got) != 2 || got[0] != "GetStream.io provider is not configured" || got[1] != "Missing GetStream.io api_key" {
		t.Fatalf("expected the deferred diagnostic, got %v", got)
	}
}

func TestProviderData_RequireClientPassesThroughWhenConfigured(t *testing.T) {
	pd := &providerData{} // client nil but configured: no diagnostic must be added
	called := false
	_ = pd.requireClient(func(summary, detail string) { called = true })
	if called {
		t.Fatal("configured provider must not emit the unconfigured diagnostic")
	}
}
