package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LivenessCheck is the real, verified result of checking whether a repo's
// claimed homepage URL actually resolves — not just whether the field is
// non-empty. A URL sitting in repo metadata is a claim; this is the check.
type LivenessCheck struct {
	URL       string // the URL actually requested, after normalization (see normalizeURL)
	IsLive    bool
	CheckedAt time.Time
	Error     string // non-empty if the check itself failed (timeout, DNS, etc.) — distinct from IsLive=false on a real HTTP error status
}

// checkTimeout is the max wait per individual attempt. Kept moderate
// rather than very long, since we retry (see maxAttempts below) instead
// of relying on one very long timeout — a few shorter attempts, spaced
// out, is more forgiving of a genuine cold-start than one long wait, and
// fails faster on a truly dead URL.
const checkTimeout = 10 * time.Second

// maxAttempts and retryDelay: a single failed request from a cold-
// starting serverless function (Vercel free tier and similar platforms
// commonly do this) is common and does NOT mean the deployment is dead —
// it means the server hasn't been asked to wake up yet. Retrying with a
// short delay gives a real, live-but-sleeping site a fair chance to
// respond before we conclude it's actually broken. Only marking IsLive
// false after ALL attempts fail is meaningfully more honest than a
// single-shot check, without needing a third-party uptime API — the
// actual bottleneck (a slow cold start) is the same regardless of which
// service sends the request.
const maxAttempts = 3
const retryDelay = 3 * time.Second

// CheckLiveness makes real HTTP requests to confirm a claimed homepage
// URL actually resolves, retrying on failure to distinguish a genuinely
// dead deployment from a slow cold start. Only meant to be called on the
// small top-K set of repos that already survived ranking — not on every
// repo a candidate owns, to keep this cheap.
func CheckLiveness(ctx context.Context, rawURL string) LivenessCheck {
	url := normalizeURL(rawURL)
	result := LivenessCheck{URL: url, CheckedAt: time.Now()}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attemptLiveness(ctx, url) {
			result.IsLive = true
			return result
		}
		lastErr = fmt.Errorf("attempt %d/%d failed", attempt, maxAttempts)

		if attempt < maxAttempts {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				result.Error = ctx.Err().Error()
				return result
			}
		}
	}

	if lastErr != nil {
		result.Error = lastErr.Error()
	}
	return result
}

// attemptLiveness makes ONE real attempt (HEAD, falling back to GET since
// some hosts reject or mishandle HEAD). Returns true only on a genuine
// 2xx/3xx response.
func attemptLiveness(ctx context.Context, url string) bool {
	attemptCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	resp, err := doRequest(attemptCtx, http.MethodHead, url)
	if err != nil {
		resp, err = doRequest(attemptCtx, http.MethodGet, url)
		if err != nil {
			return false
		}
	}
	defer resp.Body.Close()

	// 2xx and 3xx both count as "live" — a redirect (e.g. http -> https,
	// or to a login page) still means something real is running there
	return resp.StatusCode < 400
}

func doRequest(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// normalizeURL fixes a real, observed data quirk: GitHub's HomepageURL
// field sometimes comes back without a scheme at all (e.g. "quorel.vercel.app"
// instead of "https://quorel.vercel.app"). Go's http.Client requires a
// scheme to make a request at all — without this fix, a schemeless but
// genuinely live URL fails with "unsupported protocol scheme" and gets
// wrongly recorded as dead, which is a data-quality bug, not a real
// liveness problem.
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "https://" + trimmed
}
