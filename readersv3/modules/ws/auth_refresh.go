package ws

import (
	"context"
	"time"
)

// Renew on the existing connection so connection-bound event subscriptions survive.
func (m *Module) renewAuthentication(ctx context.Context, expires time.Time, token func(context.Context) (string, error)) {
	retry := false
	for {
		remaining := time.Until(expires)
		if remaining <= 0 {
			return
		}
		advance := time.Minute
		if remaining/2 < advance {
			advance = remaining / 2
		}
		delay := remaining - advance
		if retry {
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
		jwt, err := token(attempt)
		var result map[string]interface{}
		if err == nil {
			result, err = m.Exchange(attempt, Envelope{Type: "auth.refresh", Payload: map[string]interface{}{"token": jwt}})
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		next, parseErr := time.Parse(time.RFC3339Nano, asString(result["expires_at"]))
		if err != nil || parseErr != nil || !next.After(expires) {
			retry = true
			m.rt.Logf("WSS authentication renewal failed; retrying before current lease expires")
			continue
		}
		expires = next
		retry = false
	}
}
