// White-box tests for the check_timeout_budget convenience (admin.drain / ping.once /
// ping.stream): a request whose budget exceeds the client's fixed HTTP timeout fails
// fast, but the check is a no-op when the client has no fixed timeout (ctx-deadline
// default). inv-budget-exceeds-client-timeout-fails-fast.
package goodmem

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func clientWithTimeout(t *testing.T, d time.Duration) *Client {
	t.Helper()
	c, err := New("http://example.invalid", "k", WithHTTPClient(&http.Client{Timeout: d}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func i32(v int32) *int32 { return &v }
func tb(v bool) *bool    { return &v }

func TestCheckTimeoutBudget_NoClientTimeoutIsNoop(t *testing.T) {
	// Timeout 0 == no client-side ceiling (the Go ctx-deadline default): never errors,
	// even for an enormous budget.
	c := clientWithTimeout(t, 0)
	if err := checkDrainTimeoutBudget(c, &models.AdminDrainRequest{
		WaitForQuiesce: tb(true), TimeoutSec: i32(100000),
	}); err != nil {
		t.Errorf("expected no error with no client timeout, got %v", err)
	}
}

func TestCheckDrainTimeoutBudget(t *testing.T) {
	c := clientWithTimeout(t, 30*time.Second)

	// wait_for_quiesce + a 900s budget (+30 headroom) >> 30s client timeout → error.
	err := checkDrainTimeoutBudget(c, &models.AdminDrainRequest{
		WaitForQuiesce: tb(true), TimeoutSec: i32(900),
	})
	if err == nil || !strings.Contains(err.Error(), "admin.drain") || !strings.Contains(err.Error(), ">= 930s") {
		t.Errorf("want a 'requires >= 930s' error, got %v", err)
	}

	// No wait_for_quiesce → the gate skips the check entirely.
	if err := checkDrainTimeoutBudget(c, &models.AdminDrainRequest{TimeoutSec: i32(900)}); err != nil {
		t.Errorf("gate off (no wait): want no error, got %v", err)
	}

	// wait_for_quiesce with no TimeoutSec → uses the 900s IR default → still errors.
	if err := checkDrainTimeoutBudget(c, &models.AdminDrainRequest{WaitForQuiesce: tb(true)}); err == nil {
		t.Errorf("default 900s budget should exceed 30s client timeout")
	}

	// A budget that fits under the client timeout passes.
	big := clientWithTimeout(t, 20*time.Minute)
	if err := checkDrainTimeoutBudget(big, &models.AdminDrainRequest{WaitForQuiesce: tb(true), TimeoutSec: i32(900)}); err != nil {
		t.Errorf("900s budget under a 20m timeout should pass, got %v", err)
	}
}

func TestCheckPingTimeoutBudget(t *testing.T) {
	c := clientWithTimeout(t, 5*time.Second)

	// ping.once with a 60s (60000ms) budget vs 5s client timeout → error.
	if err := checkPingOnceTimeoutBudget(c, &models.PingOnceRequest{TargetID: "x", TimeoutMs: i32(60000)}); err == nil {
		t.Errorf("60s ping budget should exceed 5s client timeout")
	}
	// No TimeoutMs → no IR default → check is skipped.
	if err := checkPingOnceTimeoutBudget(c, &models.PingOnceRequest{TargetID: "x"}); err != nil {
		t.Errorf("no timeout set: want no error, got %v", err)
	}
	// ping.stream behaves the same.
	if err := checkPingStreamTimeoutBudget(c, &models.PingStreamRequest{TargetID: "x", TimeoutMs: i32(60000)}); err == nil {
		t.Errorf("60s ping.stream budget should exceed 5s client timeout")
	}
	if err := checkPingStreamTimeoutBudget(c, &models.PingStreamRequest{TargetID: "x"}); err != nil {
		t.Errorf("no timeout set: want no error, got %v", err)
	}
}
