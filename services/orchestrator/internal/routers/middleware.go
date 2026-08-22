package routers

import (
	"net/http"
	"strings"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/auth"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
)

// requireServiceToken guards endpoints that other services call directly rather
// than through the API Gateway — the worker status callback in particular.
//
// It accepts a signed, unexpired JWT carrying the service role. It previously
// compared a static shared secret, which never expired, was identical for every
// caller, and could not be rotated without restarting everything at once.
//
// A user's token is rejected even if it is an admin's: reporting delivery
// outcomes is not something a person should be able to do by hand.
func requireServiceToken(verifier *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				unauthorized(w, "missing bearer token")
				return
			}

			if _, err := verifier.VerifyService(token); err != nil {
				unauthorized(w, err.Error())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")

	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}

	return strings.TrimSpace(header[len(prefix):])
}

func unauthorized(w http.ResponseWriter, reason string) {
	rb := utils.WriteResponseFailed(nil, reason, "Unauthorized", nil)
	utils.WriteJson(w, http.StatusUnauthorized, rb)
}
