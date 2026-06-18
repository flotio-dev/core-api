package googleplay

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
)

// Publication error reasons, surfaced to callers for actionable handling.
const (
	ReasonAuth            = "auth"             // invalid service account credentials
	ReasonPermission      = "permission"       // SA not granted access / rights not propagated
	ReasonVersionConflict = "version_conflict" // versionCode already used
	ReasonInvalidBundle   = "invalid_bundle"   // AAB invalid or not signed
	ReasonTransient       = "transient"        // retryable (rate limit / 5xx)
	ReasonUnknown         = "unknown"
)

// PublishError wraps a Google Play API error with an actionable reason.
type PublishError struct {
	Reason string
	Msg    string
	Err    error
}

func (e *PublishError) Error() string { return e.Msg }
func (e *PublishError) Unwrap() error { return e.Err }

// classifyError maps a Google Play API error to a PublishError with an
// actionable reason. Non-API errors (e.g. local validation) are returned
// unchanged.
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return err
	}

	switch {
	case gerr.Code == http.StatusUnauthorized:
		return &PublishError{ReasonAuth, "invalid Google Play service account credentials", err}
	case gerr.Code == http.StatusForbidden:
		return &PublishError{ReasonPermission, "service account lacks access to the app; check the Play Console invitation and that permissions have propagated (up to ~24h)", err}
	case gerr.Code == http.StatusConflict:
		return &PublishError{ReasonVersionConflict, "versionCode already used for this app", err}
	case gerr.Code == http.StatusBadRequest:
		if mentionsVersionCode(gerr) {
			return &PublishError{ReasonVersionConflict, "versionCode already used or not strictly increasing", err}
		}
		return &PublishError{ReasonInvalidBundle, "the AAB is invalid or not signed with the upload key", err}
	case gerr.Code == http.StatusTooManyRequests || gerr.Code >= 500:
		return &PublishError{ReasonTransient, "transient Google Play API error, retry later", err}
	default:
		return &PublishError{ReasonUnknown, gerr.Message, err}
	}
}

func mentionsVersionCode(gerr *googleapi.Error) bool {
	haystack := strings.ToLower(gerr.Message + " " + gerr.Body)
	return strings.Contains(haystack, "version code") || strings.Contains(haystack, "versioncode")
}

// isRetryable reports whether err is a transient Google Play error.
func isRetryable(err error) bool {
	var pe *PublishError
	if errors.As(classifyError(err), &pe) {
		return pe.Reason == ReasonTransient
	}
	return false
}

const (
	maxPublishAttempts = 3
	retryBaseDelay     = time.Second
)

// withRetry runs fn, retrying transient errors with exponential backoff up to
// maxAttempts. It is used only for idempotent calls (not the streaming upload).
func withRetry(ctx context.Context, maxAttempts int, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !isRetryable(err) || attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryBaseDelay << (attempt - 1)):
		}
	}
	return err
}
