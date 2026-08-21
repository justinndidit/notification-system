package routers

import (
	"crypto/subtle"
	"net/http"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
)

// requireServiceToken guards endpoints that other services call directly rather
// than through the API Gateway — the worker status callback in particular.
//
// Without this, anything able to reach the orchestrator could mark an arbitrary
// notification delivered. Compared with constant time so the check does not leak
// the token a byte at a time.
func requireServiceToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := r.Header.Get("X-Service-Token")

			if len(presented) != len(expected) ||
				subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
				rb := utils.WriteResponseFailed(nil, "invalid or missing service token", "Unauthorized", nil)
				utils.WriteJson(w, http.StatusUnauthorized, rb)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
