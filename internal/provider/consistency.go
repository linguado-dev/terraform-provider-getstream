package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GetStream's reads lag its writes (#17). Create and Update record the write time and a
// fingerprint of what they wrote in private state; a Read inside consistencyWindow polls
// until the API returns that fingerprint. Later reads are single-shot, so drift isn't delayed.
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
	Unix int64  `json:"t"`
	Want string `json:"want"`
}

// markWritten records the write. Best effort: without the record, Read just doesn't poll.
func markWritten(ctx context.Context, p privateState, now time.Time, want string) {
	if p == nil {
		return
	}
	b, _ := json.Marshal(lastWrite{Unix: now.Unix(), Want: want})
	if d := p.SetKey(ctx, lastWriteKey, b); d.HasError() {
		tflog.Debug(ctx, "could not record last write in private state", map[string]any{"diagnostics": d})
	}
}

// recentWrite returns the fingerprint of our last write if it is inside consistencyWindow.
func recentWrite(ctx context.Context, p privateState, now time.Time) (string, bool) {
	if p == nil {
		return "", false
	}
	b, d := p.GetKey(ctx, lastWriteKey)
	if d.HasError() || len(b) == 0 {
		return "", false
	}
	var lw lastWrite
	if json.Unmarshal(b, &lw) != nil || lw.Unix == 0 || lw.Want == "" {
		return "", false
	}
	age := now.Sub(time.Unix(lw.Unix, 0))
	return lw.Want, age >= -time.Minute && age < consistencyWindow // allow a little clock skew
}

// fingerprint hashes attribute values into a short, order-stable string.
func fingerprint(vals ...attr.Value) string {
	h := sha256.New()
	for _, v := range vals {
		fmt.Fprintf(h, "%s\x00", v.String())
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pollConsistent calls read until accept returns true or consistencyBudget runs out;
// each read is bounded by the budget too. A not-found is retried like a stale value. On
// exhaustion it returns the latest successful read if there was one, else the last error;
// a read cut by the budget returns its timeout rather than an earlier not-found.
// Cancellation of ctx is returned as an error, never as a read result.
func pollConsistent[T any](ctx context.Context, read func(context.Context) (T, error), accept func(T) bool) (T, error) {
	var zero T
	pctx, cancel := context.WithTimeout(ctx, consistencyBudget)
	defer cancel()

	var (
		seenOK  bool
		okVal   T
		lastErr error
	)
	wait := consistencyBackoff
	for {
		v, err := read(pctx)
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		switch {
		case err != nil && errors.Is(err, context.DeadlineExceeded) && pctx.Err() != nil:
			// This read was cut by the budget: fall back to a seen read, never to an older 404.
			if seenOK {
				return okVal, nil
			}
			return zero, err
		case err == nil && accept(v):
			return v, nil
		case err == nil:
			seenOK, okVal = true, v
		case !isNotFound(err):
			return zero, err
		default:
			lastErr = err
		}

		deadline, _ := pctx.Deadline()
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(min(wait, left)):
		}
		if pctx.Err() != nil {
			break // budget spent while waiting: report what the completed reads saw
		}
		wait = min(wait*2, consistencyMaxWait)
	}
	if err := callerDone(ctx); err != nil {
		return zero, err // the caller gave up; don't pass off a stored read as the answer
	}
	if seenOK {
		return okVal, nil
	}
	return zero, lastErr
}

// callerDone is ctx.Err(), plus a deadline already past whose timer hasn't fired yet.
func callerDone(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d, ok := ctx.Deadline(); ok && !time.Now().Before(d) {
		return context.DeadlineExceeded
	}
	return nil
}
