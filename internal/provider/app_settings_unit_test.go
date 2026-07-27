package provider

import (
	"context"
	"testing"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestModelToAppSettings_OnlyKnownFields(t *testing.T) {
	t.Parallel()
	data := appSettingsResourceModel{
		SqsURL:                 types.StringValue("https://sqs.example/q"),
		SqsSecret:              types.StringValue("shh"),
		WebhookURL:             types.StringValue("https://hook.example"),
		MultiTenantEnabled:     types.BoolValue(true),
		RemindersInterval:      types.Int64Value(60),
		ImageModerationEnabled: types.BoolValue(true),
		// everything else null -> must not be set
		FileUploadConfig:  types.ObjectNull(uploadConfigAttrTypes),
		ImageUploadConfig: types.ObjectNull(uploadConfigAttrTypes),
	}
	var s stream.AppSettings
	diags := modelToAppSettings(context.Background(), data, &s)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s.SqsURL == nil || *s.SqsURL != "https://sqs.example/q" {
		t.Errorf("SqsURL not set: %v", s.SqsURL)
	}
	if s.SqsSecret == nil || *s.SqsSecret != "shh" {
		t.Errorf("SqsSecret not set: %v", s.SqsSecret)
	}
	if s.WebhookURL == nil || *s.WebhookURL != "https://hook.example" {
		t.Errorf("WebhookURL not set")
	}
	if s.MultiTenantEnabled == nil || !*s.MultiTenantEnabled {
		t.Errorf("MultiTenantEnabled not set")
	}
	if s.RemindersInterval != 60 {
		t.Errorf("RemindersInterval = %d, want 60", s.RemindersInterval)
	}
	// Unset fields must remain nil so GetStream keeps current values.
	if s.SnsTopicArn != nil {
		t.Errorf("unset SnsTopicArn leaked: %v", s.SnsTopicArn)
	}
	if s.CustomActionHandlerURL != nil {
		t.Errorf("unset CustomActionHandlerURL leaked")
	}
	if s.FileUploadConfig != nil {
		t.Errorf("unset FileUploadConfig leaked")
	}
}

func TestModelToAppSettings_UploadConfig(t *testing.T) {
	t.Parallel()
	obj, d := uploadConfigToObject(context.Background(), &stream.FileUploadConfig{
		AllowedFileExtensions: []string{".pdf", ".png"},
		BlockedMimeTypes:      []string{"application/x-msdownload"},
	})
	if d.HasError() {
		t.Fatal(d)
	}
	data := appSettingsResourceModel{
		FileUploadConfig:  obj,
		ImageUploadConfig: types.ObjectNull(uploadConfigAttrTypes),
	}
	var s stream.AppSettings
	diags := modelToAppSettings(context.Background(), data, &s)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s.FileUploadConfig == nil {
		t.Fatal("FileUploadConfig should be set")
	}
	if len(s.FileUploadConfig.AllowedFileExtensions) != 2 || s.FileUploadConfig.AllowedFileExtensions[0] != ".pdf" {
		t.Errorf("AllowedFileExtensions = %v", s.FileUploadConfig.AllowedFileExtensions)
	}
	if len(s.FileUploadConfig.BlockedMimeTypes) != 1 {
		t.Errorf("BlockedMimeTypes = %v", s.FileUploadConfig.BlockedMimeTypes)
	}
}

// TestMapAppSettingsToModel_PreservesSecrets verifies a read refresh updates
// returned fields but does NOT overwrite the write-only secrets already in state.
func TestMapAppSettingsToModel_PreservesSecrets(t *testing.T) {
	t.Parallel()
	// Pre-set managed fields (non-null) so the state-aware refresh updates them;
	// secrets are managed but must NOT be refreshed from the (secret-less) response.
	data := appSettingsResourceModel{
		SqsURL:             types.StringValue("old"),
		SqsSecret:          types.StringValue("kept-from-config"),
		SnsSecret:          types.StringValue("also-kept"),
		MultiTenantEnabled: types.BoolValue(false),
	}
	url := "https://sqs.example/q"
	mt := true
	s := &stream.AppSettings{
		SqsURL:             &url,
		MultiTenantEnabled: &mt,
		// API never returns secrets, so they are absent here.
	}
	diags := mapAppSettingsToModel(context.Background(), s, &data)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if data.Id.ValueString() != appSettingsID {
		t.Errorf("Id = %q", data.Id.ValueString())
	}
	if data.SqsURL.ValueString() != url {
		t.Errorf("SqsURL not refreshed: %q", data.SqsURL.ValueString())
	}
	if !data.MultiTenantEnabled.ValueBool() {
		t.Errorf("MultiTenantEnabled not refreshed")
	}
	// The crucial assertion: secrets kept from state, not blanked.
	if data.SqsSecret.ValueString() != "kept-from-config" {
		t.Errorf("SqsSecret was overwritten: %q", data.SqsSecret.ValueString())
	}
	if data.SnsSecret.ValueString() != "also-kept" {
		t.Errorf("SnsSecret was overwritten: %q", data.SnsSecret.ValueString())
	}
}

func TestPtrHelpers(t *testing.T) {
	t.Parallel()
	if stringFromPtr(nil).IsNull() != true {
		t.Error("nil *string should map to null")
	}
	if got := stringFromPtr(stringPtr("x")); got.ValueString() != "x" {
		t.Errorf("stringFromPtr = %q", got.ValueString())
	}
	if boolFromPtr(nil).IsNull() != true {
		t.Error("nil *bool should map to null")
	}
	if got := boolFromPtr(boolPtr(true)); !got.ValueBool() {
		t.Error("boolFromPtr(true) should be true")
	}
}
