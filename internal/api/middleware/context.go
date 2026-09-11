package middleware

import "context"

type contextKey string

const RecruiterIDKey contextKey = "recruiter_id"

func RecruiterIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(RecruiterIDKey).(string)
	return id, ok
}
