package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// An environment may declare the provider with no credentials while managing
// zero GetStream resources (dev → qa → prod rollout). Configure must succeed and
// hand resources a deferred state; the ORIGINAL diagnostics surface only when a
// resource asks for the client.

func emptyProviderConfig(t *testing.T, p provider.Provider) tfsdk.Config {
	t.Helper()
	var sr provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		t.Fatalf("schema: %v", sr.Diagnostics)
	}
	// An object whose attributes are all null == a `provider "getstream" {}` block.
	objType := sr.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	attrs := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		attrs[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.Config{Schema: sr.Schema, Raw: tftypes.NewValue(objType, attrs)}
}

func TestProviderConfigure_NoCredentialsIsDeferredNotFatal(t *testing.T) {
	for _, k := range append(append([]string{}, envAPIKeyNames...), envAPISecretNames...) {
		t.Setenv(k, "")
	}
	p := New("test")()
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{Config: emptyProviderConfig(t, p)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure must not error without credentials, got %v", resp.Diagnostics)
	}
	pd, ok := resp.ResourceData.(*providerData)
	if !ok || pd == nil {
		t.Fatalf("expected *providerData in ResourceData, got %T", resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Fatal("DataSourceData must carry the same deferred state as ResourceData")
	}
	if len(pd.deferredErrors) != 2 {
		t.Fatalf("expected both missing-credential diagnostics deferred, got %+v", pd.deferredErrors)
	}
	var got []string
	if c := pd.requireClient(func(summary, detail string) { got = append(got, summary) }); c != nil {
		t.Fatalf("expected nil client, got %v", c)
	}
	want := []string{"Missing GetStream.io API key", "Missing GetStream.io API secret"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("deferred diagnostics must keep the original summaries; got %v", got)
	}
}

func TestProviderConfigure_OnlySecretMissingDefersOneError(t *testing.T) {
	for _, k := range append(append([]string{}, envAPIKeyNames...), envAPISecretNames...) {
		t.Setenv(k, "")
	}
	t.Setenv(envAPIKeyNames[0], "key-present")
	p := New("test")()
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{Config: emptyProviderConfig(t, p)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	pd := resp.ResourceData.(*providerData)
	if len(pd.deferredErrors) != 1 || pd.deferredErrors[0].summary != "Missing GetStream.io API secret" {
		t.Fatalf("expected exactly the missing-secret diagnostic, got %+v", pd.deferredErrors)
	}
}

func TestProviderData_RequireClientPassesThroughWhenConfigured(t *testing.T) {
	pd := &providerData{} // configured (client set by Configure in real runs): no diagnostic
	called := false
	_ = pd.requireClient(func(summary, detail string) { called = true })
	if called {
		t.Fatal("configured provider must not emit deferred diagnostics")
	}
}
