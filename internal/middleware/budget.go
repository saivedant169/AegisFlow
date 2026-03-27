package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/aegisflow/aegisflow/pkg/types"
)

// BudgetCheckFunc checks whether a tenant's budget allows the request.
// Returns allowed status, any warning messages, and a block message if denied.
type BudgetCheckFunc func(tenantID, model string) (allowed bool, warnings []string, blockMsg string)

// BudgetCheck returns middleware that enforces spend budgets per tenant.
// Requests that exceed the budget receive a 429 response; requests approaching
// the limit receive X-AegisFlow-Budget-Warning headers.
func BudgetCheck(checkFn BudgetCheckFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checkFn == nil || r.URL.Path == "/health" || r.URL.Path != "/v1/chat/completions" {
				next.ServeHTTP(w, r)
				return
			}

			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check tenant-level budget. Model-specific checks happen after
			// the handler parses the request body.
			allowed, warnings, blockMsg := checkFn(tenant.ID, "*")

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(types.NewErrorResponse(429, "budget_exceeded", blockMsg))
				return
			}

			for _, warning := range warnings {
				w.Header().Add("X-AegisFlow-Budget-Warning", warning)
			}

			next.ServeHTTP(w, r)
		})
	}
}
