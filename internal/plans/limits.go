package plans

import (
	"context"
	"fmt"
	"time"
)

const (
	ActionOutreach = "outreach"
	ActionRescan   = "rescan"
)

const dayWindow = 24 * time.Hour

type DenyReason string

const (
	DenyThrottled     DenyReason = "throttled"
	DenyDailyLimit    DenyReason = "daily_limit"
	DenyLifetimeLimit DenyReason = "lifetime_limit"
)

type Decision struct {
	Allowed    bool
	Reason     DenyReason
	RetryAfter time.Duration
	Message    string
}

type Usage interface {
	LastUsageAt(ctx context.Context, candidateID string) (*time.Time, error)
	CountUsageSince(ctx context.Context, candidateID, action string, since time.Time) (int, time.Time, error)
	CountDistinctCandidates(ctx context.Context, recruiterID, action string) (int, error)
	CandidateHasUsage(ctx context.Context, recruiterID, candidateID, action string) (bool, error)
}

func Check(ctx context.Context, usage Usage, plan Plan, recruiterID, candidateID, action string, now time.Time) (Decision, error) {
	if plan.MinInterval > 0 {
		last, err := usage.LastUsageAt(ctx, candidateID)
		if err != nil {
			return Decision{}, err
		}
		if last != nil {
			wait := plan.MinInterval - now.Sub(*last)
			if wait > 0 {
				return Decision{
					Reason:     DenyThrottled,
					RetryAfter: wait,
					Message:    fmt.Sprintf("please wait %s before acting on this candidate again", roundUp(wait)),
				}, nil
			}
		}
	}

	dailyLimit, lifetimeLimit := limitsFor(plan, action)

	if dailyLimit != Unlimited {
		count, oldest, err := usage.CountUsageSince(ctx, candidateID, action, now.Add(-dayWindow))
		if err != nil {
			return Decision{}, err
		}
		if Exceeds(dailyLimit, count) {
			wait := oldest.Add(dayWindow).Sub(now)
			return Decision{
				Reason:     DenyDailyLimit,
				RetryAfter: wait,
				Message:    fmt.Sprintf("daily limit of %d reached for this candidate, try again in %s", dailyLimit, roundUp(wait)),
			}, nil
		}
	}

	if lifetimeLimit != Unlimited {
		already, err := usage.CandidateHasUsage(ctx, recruiterID, candidateID, action)
		if err != nil {
			return Decision{}, err
		}
		if !already {
			used, err := usage.CountDistinctCandidates(ctx, recruiterID, action)
			if err != nil {
				return Decision{}, err
			}
			if Exceeds(lifetimeLimit, used) {
				return Decision{
					Reason:  DenyLifetimeLimit,
					Message: fmt.Sprintf("your plan allows %s for %d candidates, upgrade to continue", action, lifetimeLimit),
				}, nil
			}
		}
	}

	return Decision{Allowed: true}, nil
}

func limitsFor(plan Plan, action string) (daily, lifetime int) {
	if action == ActionRescan {
		return plan.RescanPerCandidatePerDay, plan.RescanCandidatesLifetime
	}
	return plan.OutreachGenerationsPerDay, plan.OutreachCandidatesLifetime
}

func roundUp(d time.Duration) time.Duration {
	if d < time.Second {
		return time.Second
	}
	return d.Round(time.Second)
}
