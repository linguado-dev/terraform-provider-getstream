package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

type fakePrivate map[string][]byte

func (f fakePrivate) GetKey(_ context.Context, k string) ([]byte, diag.Diagnostics) {
	return f[k], nil
}

func (f fakePrivate) SetKey(_ context.Context, k string, v []byte) diag.Diagnostics {
	f[k] = v
	return nil
}

// fastPolling shrinks the poll timings for the duration of a test.
func fastPolling(t *testing.T) {
	t.Helper()
	b, w, m := consistencyBudget, consistencyBackoff, consistencyMaxWait
	consistencyBudget, consistencyBackoff, consistencyMaxWait = 200*time.Millisecond, time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { consistencyBudget, consistencyBackoff, consistencyMaxWait = b, w, m })
}

func TestRecentlyWritten(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	p := fakePrivate{}

	if recentlyWritten(ctx, p, now) {
		t.Fatal("no stamp (e.g. after import) must not poll")
	}
	if recentlyWritten(ctx, nil, now) {
		t.Fatal("nil private state must not poll")
	}
	markWritten(ctx, p, now)
	if !recentlyWritten(ctx, p, now.Add(time.Second)) {
		t.Fatal("a read right after our write must poll")
	}
	if recentlyWritten(ctx, p, now.Add(consistencyWindow+time.Second)) {
		t.Fatal("a read after the window must be single-shot")
	}
	p[lastWriteKey] = []byte("not json")
	if recentlyWritten(ctx, p, now) {
		t.Fatal("a corrupt stamp must not poll")
	}
}

func TestPollConsistent(t *testing.T) {
	fastPolling(t)
	ctx := context.Background()
	notFound := stream.Error{StatusCode: http.StatusNotFound}
	fatal := errors.New("boom")

	cases := []struct {
		name      string
		seq       []any // int values or errors, returned in order; the last repeats
		wantVal   int
		wantErr   error
		wantCalls int // 0 = at least 2 (budget exhausted)
	}{
		{"consistent first read", []any{2}, 2, nil, 1},
		{"stale then fresh", []any{1, 1, 2}, 2, nil, 3},
		{"not found then fresh", []any{notFound, 2}, 2, nil, 2},
		{"other error is not retried", []any{fatal}, 0, fatal, 1},
		{"stale forever returns last read", []any{1}, 1, nil, 0},
		{"not found forever returns not found", []any{notFound}, 0, notFound, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			read := func() (int, error) {
				v := tc.seq[min(calls, len(tc.seq)-1)]
				calls++
				if err, ok := v.(error); ok {
					return 0, err
				}
				return v.(int), nil
			}
			start := time.Now()
			got, err := pollConsistent(ctx, read, func(v int) bool { return v == 2 })
			sameErr := errors.Is(err, tc.wantErr) || (isNotFound(tc.wantErr) && isNotFound(err))
			if got != tc.wantVal || !sameErr {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, err, tc.wantVal, tc.wantErr)
			}
			if tc.wantCalls > 0 && calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantCalls == 0 && calls < 2 {
				t.Fatalf("calls = %d, want a retry before giving up", calls)
			}
			if time.Since(start) > consistencyBudget+100*time.Millisecond {
				t.Fatalf("poll overran its budget: %s", time.Since(start))
			}
		})
	}
}

func TestChannelTypeMatches(t *testing.T) {
	ctx := context.Background()
	ct := stream.NewChannelType("msg")
	ct.MaxMessageLength = 2000
	ct.Reactions = false

	var want channelTypeResourceModel
	if d := mapChannelTypeToModel(ctx, ct, &want); d.HasError() {
		t.Fatal(d)
	}
	resp := &stream.GetChannelTypeResponse{ChannelType: ct}
	if !channelTypeMatches(ctx, resp, want) {
		t.Fatal("identical read must match")
	}

	stale := *ct
	stale.MaxMessageLength = 1000
	if channelTypeMatches(ctx, &stream.GetChannelTypeResponse{ChannelType: &stale}, want) {
		t.Fatal("stale max_message_length must not match")
	}
	if channelTypeMatches(ctx, nil, want) {
		t.Fatal("nil response must not match")
	}
}
