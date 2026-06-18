package googleplay

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"
)

func classify(t *testing.T, code int, msg string) *PublishError {
	t.Helper()
	// Wrap like the API methods do, to ensure errors.As unwraps correctly.
	wrapped := fmt.Errorf("googleplay: call: %w", &googleapi.Error{Code: code, Message: msg})
	var pe *PublishError
	if !errors.As(classifyError(wrapped), &pe) {
		t.Fatalf("expected *PublishError for code %d", code)
	}
	return pe
}

func TestClassifyError(t *testing.T) {
	if got := classify(t, 401, "").Reason; got != ReasonAuth {
		t.Fatalf("401 -> %q want %q", got, ReasonAuth)
	}
	if got := classify(t, 403, "").Reason; got != ReasonPermission {
		t.Fatalf("403 -> %q want %q", got, ReasonPermission)
	}
	if got := classify(t, 409, "").Reason; got != ReasonVersionConflict {
		t.Fatalf("409 -> %q want %q", got, ReasonVersionConflict)
	}
	if got := classify(t, 400, "APK specifies a version code that has already been used").Reason; got != ReasonVersionConflict {
		t.Fatalf("400+versioncode -> %q want %q", got, ReasonVersionConflict)
	}
	if got := classify(t, 400, "bundle not signed").Reason; got != ReasonInvalidBundle {
		t.Fatalf("400 -> %q want %q", got, ReasonInvalidBundle)
	}
	if got := classify(t, 429, "").Reason; got != ReasonTransient {
		t.Fatalf("429 -> %q want %q", got, ReasonTransient)
	}
	if got := classify(t, 503, "").Reason; got != ReasonTransient {
		t.Fatalf("503 -> %q want %q", got, ReasonTransient)
	}
}

func TestClassifyErrorPassThrough(t *testing.T) {
	local := errors.New("googleplay: empty track")
	if got := classifyError(local); got != local {
		t.Fatalf("non-API error should pass through unchanged, got %v", got)
	}
	if classifyError(nil) != nil {
		t.Fatal("nil should classify to nil")
	}
}

func TestIsRetryable(t *testing.T) {
	transient := fmt.Errorf("x: %w", &googleapi.Error{Code: 503})
	if !isRetryable(transient) {
		t.Fatal("503 should be retryable")
	}
	conflict := fmt.Errorf("x: %w", &googleapi.Error{Code: 409})
	if isRetryable(conflict) {
		t.Fatal("409 should not be retryable")
	}
	if isRetryable(errors.New("local")) {
		t.Fatal("local error should not be retryable")
	}
}
