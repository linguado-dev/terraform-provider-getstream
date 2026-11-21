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

func TestRecentWrite(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	p := fakePrivate{}

	if _, ok := recentWrite(ctx, p, now); ok {
		t.Fatal("no record (e.g. after import) must not poll")
	}
	if _, ok := recentWrite(ctx, nil, now); ok {
		t.Fatal("nil private state must not poll")
	}
	markWritten(ctx, p, now, "fp")
	if want, ok := recentWrite(ctx, p, now.Add(time.Second)); !ok || want != "fp" {
		t.Fatalf("a read right after our write must poll for our fingerprint, got (%q, %v)", want, ok)
	}
	if _, ok := recentWrite(ctx, p, now.Add(consistencyWindow+time.Second)); ok {
		t.Fatal("a read after the window must be single-shot")
	}
	p[lastWriteKey] = []byte("not json")
	if _, ok := recentWrite(ctx, p, now); ok {
		t.Fatal("a corrupt record must not poll")
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
		{"a seen read beats later not-founds", []any{1, notFound}, 1, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			read := func(context.Context) (int, error) {
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

func TestPollConsistent_SlowReadIsBounded(t *testing.T) {
	fastPolling(t)
	calls := 0
	read := func(ctx context.Context) (int, error) {
		calls++
		if calls == 1 {
			return 1, nil // stale
		}
		<-ctx.Done() // a GET that would hang past the budget
		return 0, ctx.Err()
	}
	start := time.Now()
	got, err := pollConsistent(context.Background(), read, func(v int) bool { return v == 2 })
	if got != 1 || err != nil {
		t.Fatalf("got (%d, %v), want the stale read (1, nil)", got, err)
	}
	if time.Since(start) > consistencyBudget+100*time.Millisecond {
		t.Fatalf("a hanging read overran the budget: %s", time.Since(start))
	}
}

func TestPollConsistent_CancelIsAnError(t *testing.T) {
	fastPolling(t)
	ctx, cancel := context.WithCancel(context.Background())
	notFound := stream.Error{StatusCode: http.StatusNotFound}
	read := func(context.Context) (int, error) {
		cancel()
		return 0, notFound
	}
	_, err := pollConsistent(ctx, read, func(int) bool { return false })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled (not a not-found that would drop the resource)", err)
	}
}

func TestChannelTypeMatches(t *testing.T) {
	ctx := context.Background()
	ct := stream.NewChannelType("msg")
	ct.MaxMessageLength = 2000
	ct.Reactions = false

	var written channelTypeResourceModel
	if d := mapChannelTypeToModel(ctx, ct, &written); d.HasError() {
		t.Fatal(d)
	}
	want := channelTypeFingerprint(written)
	if !channelTypeMatches(ctx, &stream.GetChannelTypeResponse{ChannelType: ct}, want) {
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
