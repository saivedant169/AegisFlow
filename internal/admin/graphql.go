package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	gqlerrors "github.com/graphql-go/graphql/gqlerrors"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/middleware"
)

// buildSchema constructs the GraphQL schema that mirrors the REST admin API.
// Every resolver delegates to the same provider interfaces that the REST
// handlers use, so no new data sources are introduced.
func (s *Server) buildSchema() (graphql.Schema, error) {

	// ---------- Query ----------

	queryFields := graphql.Fields{
		"usage": &graphql.Field{
			Type: graphql.NewObject(graphql.ObjectConfig{
				Name:   "UsageMap",
				Fields: graphql.Fields{"raw": &graphql.Field{Type: JSONScalar}},
			}),
			Args: graphql.FieldConfigArgument{
				"tenantId": &graphql.ArgumentConfig{Type: graphql.String},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				all := s.tracker.GetAllUsage()
				if tid, ok := p.Args["tenantId"].(string); ok && tid != "" {
					if tu := all[tid]; tu != nil {
						return map[string]interface{}{"raw": tu}, nil
					}
					return map[string]interface{}{"raw": nil}, nil
				}
				return map[string]interface{}{"raw": all}, nil
			},
		},

		"providers": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				type providerInfo struct {
					Name    string   `json:"name"`
					Type    string   `json:"type"`
					Enabled bool     `json:"enabled"`
					BaseURL string   `json:"base_url,omitempty"`
					Models  []string `json:"models,omitempty"`
					Healthy bool     `json:"healthy"`
					Region  string   `json:"region,omitempty"`
				}
				var out []interface{}
				for _, pc := range s.cfg.Providers {
					healthy := false
					if pc.Enabled {
						if pr, err := s.registry.Get(pc.Name); err == nil {
							healthy = pr.Healthy(p.Context)
						}
					}
					out = append(out, providerInfo{
						Name:    pc.Name,
						Type:    pc.Type,
						Enabled: pc.Enabled,
						BaseURL: pc.BaseURL,
						Models:  pc.Models,
						Healthy: healthy,
						Region:  pc.Region,
					})
				}
				if out == nil {
					out = []interface{}{}
				}
				return out, nil
			},
		},

		"tenants": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				type tenantInfo struct {
					ID                string   `json:"id"`
					Name              string   `json:"name"`
					KeyCount          int      `json:"key_count"`
					RequestsPerMinute int      `json:"requests_per_minute"`
					TokensPerMinute   int      `json:"tokens_per_minute"`
					AllowedModels     []string `json:"allowed_models"`
				}
				var out []interface{}
				for _, t := range s.cfg.Tenants {
					out = append(out, tenantInfo{
						ID:                t.ID,
						Name:              t.Name,
						KeyCount:          len(t.APIKeys),
						RequestsPerMinute: t.RateLimit.RequestsPerMinute,
						TokensPerMinute:   t.RateLimit.TokensPerMinute,
						AllowedModels:     t.AllowedModels,
					})
				}
				if out == nil {
					out = []interface{}{}
				}
				return out, nil
			},
		},

		"policies": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				type policyInfo struct {
					Name     string   `json:"name"`
					Type     string   `json:"type"`
					Action   string   `json:"action"`
					Phase    string   `json:"phase"`
					Keywords []string `json:"keywords,omitempty"`
					Patterns []string `json:"patterns,omitempty"`
				}
				var out []interface{}
				for _, pol := range s.cfg.Policies.Input {
					out = append(out, policyInfo{
						Name: pol.Name, Type: pol.Type, Action: pol.Action, Phase: "input",
						Keywords: pol.Keywords, Patterns: pol.Patterns,
					})
				}
				for _, pol := range s.cfg.Policies.Output {
					out = append(out, policyInfo{
						Name: pol.Name, Type: pol.Type, Action: pol.Action, Phase: "output",
						Keywords: pol.Keywords, Patterns: pol.Patterns,
					})
				}
				if out == nil {
					out = []interface{}{}
				}
				return out, nil
			},
		},

		"cache": &graphql.Field{
			Type: JSONScalar,
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				if s.cache != nil {
					return s.cache.Stats(), nil
				}
				return cache.CacheStats{}, nil
			},
		},

		"violations": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				return s.requestLog.RecentViolations(100), nil
			},
		},

		"analytics": &graphql.Field{
			Type: JSONScalar,
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				if s.analyticsProvider == nil {
					return map[string]interface{}{"error": "analytics not enabled"}, nil
				}
				return map[string]interface{}{
					"dimensions": s.analyticsProvider.Dimensions(),
					"summary":    s.analyticsProvider.RealtimeSummary(),
				}, nil
			},
		},

		"alerts": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Args: graphql.FieldConfigArgument{
				"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 100},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.analyticsProvider == nil {
					return []interface{}{}, nil
				}
				limit := 100
				if v, ok := p.Args["limit"].(int); ok && v > 0 {
					limit = v
				}
				return s.analyticsProvider.RecentAlerts(limit), nil
			},
		},

		"budgets": &graphql.Field{
			Type: JSONScalar,
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				if s.budgetProvider == nil {
					return map[string]interface{}{
						"statuses":  []interface{}{},
						"forecasts": []interface{}{},
					}, nil
				}
				return map[string]interface{}{
					"statuses":  s.budgetProvider.AllStatuses(),
					"forecasts": s.budgetProvider.ForecastAll(),
				}, nil
			},
		},

		"rollouts": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				if s.rolloutMgr == nil {
					return []interface{}{}, nil
				}
				result, err := s.rolloutMgr.ListRollouts()
				if err != nil {
					return nil, err
				}
				if result == nil {
					return []interface{}{}, nil
				}
				return result, nil
			},
		},

		"audit": &graphql.Field{
			Type: graphql.NewList(JSONScalar),
			Args: graphql.FieldConfigArgument{
				"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 100},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.auditProvider == nil {
					return []interface{}{}, nil
				}
				limit := 100
				if v, ok := p.Args["limit"].(int); ok && v > 0 {
					limit = v
				}
				return s.auditProvider.Query("", "", "", "", limit)
			},
		},

		"costRecommendations": &graphql.Field{
			Type: JSONScalar,
			Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
				if s.costOptProvider == nil {
					return map[string]interface{}{"recommendations": []interface{}{}}, nil
				}
				return map[string]interface{}{
					"recommendations": s.costOptProvider.Recommendations(),
				}, nil
			},
		},

		"whoami": &graphql.Field{
			Type: JSONScalar,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				role := middleware.RoleFromContext(p.Context)
				tenant := middleware.TenantFromContext(p.Context)
				resp := map[string]string{"role": role}
				if tenant != nil {
					resp["tenant_id"] = tenant.ID
					resp["tenant_name"] = tenant.Name
				}
				return resp, nil
			},
		},
	}

	// ---------- Mutations ----------

	mutationFields := graphql.Fields{
		"acknowledgeAlert": &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.analyticsProvider == nil {
					return false, nil
				}
				id := p.Args["id"].(string)
				return s.analyticsProvider.AcknowledgeAlert(id), nil
			},
		},

		"createRollout": &graphql.Field{
			Type: JSONScalar,
			Args: graphql.FieldConfigArgument{
				"input": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.NewInputObject(graphql.InputObjectConfig{
						Name: "RolloutInput",
						Fields: graphql.InputObjectConfigFieldMap{
							"routeModel":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
							"canaryProvider":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
							"stages":              &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.NewList(graphql.Int))},
							"observationWindow":   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
							"errorThreshold":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Float)},
							"latencyP95Threshold": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
						},
					})),
				},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.rolloutMgr == nil {
					return nil, errRolloutUnavailable
				}
				input := p.Args["input"].(map[string]interface{})

				routeModel := input["routeModel"].(string)
				canaryProvider := input["canaryProvider"].(string)
				obsWindowStr := input["observationWindow"].(string)
				errorThreshold := input["errorThreshold"].(float64)
				latencyP95 := int64(input["latencyP95Threshold"].(int))

				stagesRaw := input["stages"].([]interface{})
				stages := make([]int, len(stagesRaw))
				for i, v := range stagesRaw {
					stages[i] = v.(int)
				}

				obsWindow, err := time.ParseDuration(obsWindowStr)
				if err != nil {
					return nil, err
				}

				var baselineProviders []string
				for _, route := range s.cfg.Routes {
					if strings.EqualFold(route.Match.Model, routeModel) {
						baselineProviders = route.Providers
						break
					}
				}

				return s.rolloutMgr.CreateRollout(
					routeModel, baselineProviders, canaryProvider,
					stages, obsWindow, errorThreshold, latencyP95,
				)
			},
		},

		"pauseRollout": &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.rolloutMgr == nil {
					return false, errRolloutUnavailable
				}
				if err := s.rolloutMgr.PauseRollout(p.Args["id"].(string)); err != nil {
					return false, err
				}
				return true, nil
			},
		},

		"resumeRollout": &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.rolloutMgr == nil {
					return false, errRolloutUnavailable
				}
				if err := s.rolloutMgr.ResumeRollout(p.Args["id"].(string)); err != nil {
					return false, err
				}
				return true, nil
			},
		},

		"rollbackRollout": &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if s.rolloutMgr == nil {
					return false, errRolloutUnavailable
				}
				if err := s.rolloutMgr.RollbackRollout(p.Args["id"].(string)); err != nil {
					return false, err
				}
				return true, nil
			},
		},
	}

	return graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		}),
	})
}

// errRolloutUnavailable is returned when the rollout manager is nil.
var errRolloutUnavailable = &gqlError{msg: "rollout manager not available"}

type gqlError struct{ msg string }

func (e *gqlError) Error() string { return e.msg }

// JSONScalar is a custom scalar that passes arbitrary JSON values through.
var JSONScalar = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "JSON",
	Description: "Arbitrary JSON value",
	Serialize: func(value interface{}) interface{} {
		return value
	},
	ParseValue: func(value interface{}) interface{} {
		return value
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		return nil
	},
})

// graphqlRequest is the expected JSON body for POST /admin/v1/graphql.
type graphqlRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// graphqlHandler returns an http.HandlerFunc that executes GraphQL queries.
func (s *Server) graphqlHandler(schema graphql.Schema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON body"})
			return
		}

		result := graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  req.Query,
			VariableValues: req.Variables,
			OperationName:  req.OperationName,
			Context:        r.Context(),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// ExecuteGraphQL runs a GraphQL query directly (useful for testing).
func (s *Server) ExecuteGraphQL(ctx context.Context, query string, variables map[string]interface{}) *graphql.Result {
	schema, err := s.buildSchema()
	if err != nil {
		return &graphql.Result{Errors: []gqlerrors.FormattedError{
			{Message: err.Error()},
		}}
	}
	return graphql.Do(graphql.Params{
		Schema:         schema,
		RequestString:  query,
		VariableValues: variables,
		Context:        ctx,
	})
}
