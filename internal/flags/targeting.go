package flags

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/flags-gg/orchestrator/internal/stats"
)

// Rule targets users by an attribute of their evaluation context. Rules are checked in order and the first
// match decides the flag (Serve); with no match the flag's rollout applies. Matching ignores case.
type Rule struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"`
	Values    []string `json:"values"`
	Serve     bool     `json:"serve"`
}

// EvalContext is what an SDK knows about the user: "key" is the targeting key, other attributes are
// strings or lists of strings (e.g. "groups": ["beta", "staff"]).
type EvalContext map[string]any

const ContextHeader = "x-flags-context"

var operators = map[string]func(have, want string) bool{
	"in":          func(h, w string) bool { return h == w },
	"not_in":      func(h, w string) bool { return h == w }, // negated in Matches
	"contains":    strings.Contains,
	"starts_with": strings.HasPrefix,
	"ends_with":   strings.HasSuffix,
}

// values returns the attribute as lower-cased strings; a missing attribute has none.
func (c EvalContext) values(attr string) []string {
	switch v := c[attr].(type) {
	case string:
		return []string{strings.ToLower(v)}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, strings.ToLower(s))
			}
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, s := range v {
			out[i] = strings.ToLower(s)
		}
		return out
	}
	return nil
}

// Matches reports whether any of the user's values for the attribute matches any rule value.
// not_in matches when the user has the attribute and none of its values is listed.
// A user without the attribute never matches, whatever the operator.
func (r Rule) Matches(ctx EvalContext) bool {
	have := ctx.values(r.Attribute)
	cmp, ok := operators[r.Operator]
	if !ok || len(have) == 0 {
		return false
	}
	hit := false
	for _, h := range have {
		for _, w := range r.Values {
			if cmp(h, strings.ToLower(w)) {
				hit = true
			}
		}
	}
	if r.Operator == "not_in" {
		return !hit
	}
	return hit
}

// MatchRules returns the first matching rule's Serve value.
func MatchRules(rules []Rule, ctx EvalContext) (serve, matched bool) {
	if len(ctx) == 0 {
		return false, false
	}
	for _, r := range rules {
		if r.Matches(ctx) {
			return r.Serve, true
		}
	}
	return false, false
}

// ValidateRules bounds what the dashboard can store.
func ValidateRules(rules []Rule) error {
	if len(rules) > 50 {
		return errors.New("at most 50 rules")
	}
	for i, r := range rules {
		if r.Attribute == "" || len(r.Attribute) > 100 {
			return fmt.Errorf("rule %d: attribute must be 1-100 characters", i+1)
		}
		if _, ok := operators[r.Operator]; !ok {
			return fmt.Errorf("rule %d: unknown operator %q", i+1, r.Operator)
		}
		if len(r.Values) == 0 || len(r.Values) > 1000 {
			return fmt.Errorf("rule %d: needs 1-1000 values", i+1)
		}
		for _, v := range r.Values {
			if v == "" || len(v) > 500 {
				return fmt.Errorf("rule %d: values must be 1-500 characters", i+1)
			}
		}
	}
	return nil
}

// ParseContextHeader decodes the base64(JSON object) an SDK sends in x-flags-context.
func ParseContextHeader(h string) (EvalContext, error) {
	if h == "" {
		return nil, nil
	}
	if len(h) > 8192 {
		return nil, errors.New("context too large")
	}
	h = strings.TrimRight(h, "=")
	raw, err := base64.RawURLEncoding.DecodeString(h)
	if err != nil {
		if raw, err = base64.RawStdEncoding.DecodeString(h); err != nil {
			return nil, err
		}
	}
	var ctx EvalContext
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// Decide is what one user is served: off when disabled, else the first matching rule, else the rollout.
func Decide(f Flag, ctx EvalContext, key string) (on, ruleHit bool) {
	if !f.Enabled {
		return false, false
	}
	if serve, matched := MatchRules(f.Rules, ctx); matched {
		return serve, true
	}
	rollout := 100
	if f.Rollout != nil {
		rollout = *f.Rollout
	}
	return InRollout(f.Details.Name, key, rollout), false
}

// exposures is every flag's decision for one user, for the usage metrics; nil without a targeting key.
func exposures(flags []Flag, ctx EvalContext, key string) []stats.Exposure {
	if key == "" {
		return nil
	}
	out := make([]stats.Exposure, 0, len(flags))
	for _, f := range flags {
		if f.Details.Name == "" {
			continue
		}
		on, hit := Decide(f, ctx, key)
		out = append(out, stats.Exposure{Flag: f.Details.Name, On: on, RuleHit: hit})
	}
	return out
}
