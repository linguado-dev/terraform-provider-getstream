package provider

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GetStream's reads lag its writes (#17). Create and Update stamp the write time in
// private state; a Read inside consistencyWindow of that stamp polls until the API
// agrees with state. Later reads are single-shot, so real drift is never delayed.
var (
	consistencyWindow  = 2 * time.Minute
	consistencyBudget  = 15 * time.Second
	consistencyBackoff = 250 * time.Millisecond
	consistencyMaxWait = 2 * time.Second
)

const lastWriteKey = "last_write"

// privateState is the subset of the framework's private state used here.
type privateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

type lastWrite struct {
	Unix int64 `json:"t"`
}

// markWritten stamps the write time. Best effort: without the stamp, Read just doesn't poll.
func markWritten(ctx context.Context, p privateState, now time.Time) {
	if p == nil {
		return
	}
	b, _ := json.Marshal(lastWrite{Unix: now.Unix()})
	if d := p.SetKey(ctx, lastWriteKey, b); d.HasError() {
		tflog.Debug(ctx, "could not record last write in private state", map[string]any{"diagnostics": d})
	}
}

// recentlyWritten reports whether this provider wrote the resource within consistencyWindow.
func recentlyWritten(ctx context.Context, p privateState, now time.Time) bool {
	if p == nil {
		return false
	}
	b, d := p.GetKey(ctx, lastWriteKey)
	if d.HasError() || len(b) == 0 {
		return false
	}
	var lw lastWrite
	if json.Unmarshal(b, &lw) != nil || lw.Unix == 0 {
		return false
	}
	return now.Sub(time.Unix(lw.Unix, 0)) < consistencyWindow
}

// pollConsistent calls read until accept returns true or the budget runs out, and
// returns the last result. A not-found error is retried like a stale value.
func pollConsistent[T any](ctx context.Context, read func() (T, error), accept func(T) bool) (T, error) {
	deadline := time.Now().Add(consistencyBudget)
	wait := consistencyBackoff
	for {
		v, err := read()
		if (err == nil && accept(v)) || (err != nil && !isNotFound(err)) {
			return v, err
		}
		if time.Now().Add(wait).After(deadline) {
			return v, err
		}
		select {
		case <-ctx.Done():
			return v, err
		case <-time.After(wait):
		}
		wait = min(wait*2, consistencyMaxWait)
	}
}
