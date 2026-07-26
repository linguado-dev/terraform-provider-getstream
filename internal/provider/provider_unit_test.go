package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		v        types.String
		fallback string
		want     string
	}{
		{"config wins", types.StringValue("cfg"), "env", "cfg"},
		{"null -> fallback", types.StringNull(), "env", "env"},
		{"unknown -> fallback", types.StringUnknown(), "env", "env"},
		{"empty config -> fallback", types.StringValue(""), "env", "env"},
		{"both empty", types.StringNull(), "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := firstNonEmpty(tc.v, tc.fallback); got != tc.want {
				t.Fatalf("firstNonEmpty(%v, %q) = %q, want %q", tc.v, tc.fallback, got, tc.want)
			}
		})
	}
}
