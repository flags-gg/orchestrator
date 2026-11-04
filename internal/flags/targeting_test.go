package flags

import (
	"encoding/base64"
	"testing"
)

func TestRuleMatching(t *testing.T) {
	ctx := EvalContext{"key": "user-1", "email": "Ana@Acme.com", "groups": []any{"beta", "staff"}, "country": "GB"}
	cases := []struct {
		rule Rule
		want bool
	}{
		{Rule{Attribute: "email", Operator: "ends_with", Values: []string{"@acme.com"}}, true}, // case-insensitive
		{Rule{Attribute: "email", Operator: "ends_with", Values: []string{"@other.com"}}, false},
		{Rule{Attribute: "groups", Operator: "in", Values: []string{"beta"}}, true}, // any of a list attribute
		{Rule{Attribute: "groups", Operator: "in", Values: []string{"admin"}}, false},
		{Rule{Attribute: "country", Operator: "not_in", Values: []string{"US", "CA"}}, true},
		{Rule{Attribute: "country", Operator: "not_in", Values: []string{"gb"}}, false},
		{Rule{Attribute: "plan", Operator: "not_in", Values: []string{"free"}}, false}, // missing attribute never matches
		{Rule{Attribute: "key", Operator: "in", Values: []string{"user-1", "user-9"}}, true},
		{Rule{Attribute: "email", Operator: "contains", Values: []string{"acme"}}, true},
		{Rule{Attribute: "email", Operator: "starts_with", Values: []string{"ana@"}}, true},
		{Rule{Attribute: "email", Operator: "regex", Values: []string{".*"}}, false}, // unknown operator
	}
	for _, c := range cases {
		if got := c.rule.Matches(ctx); got != c.want {
			t.Errorf("%+v: got %v, want %v", c.rule, got, c.want)
		}
	}
}

func TestFirstMatchingRuleDecides(t *testing.T) {
	rules := []Rule{
		{Attribute: "email", Operator: "in", Values: []string{"blocked@acme.com"}, Serve: false},
		{Attribute: "email", Operator: "ends_with", Values: []string{"@acme.com"}, Serve: true},
	}
	if serve, ok := MatchRules(rules, EvalContext{"email": "blocked@acme.com"}); !ok || serve {
		t.Errorf("blocked user: serve=%v matched=%v", serve, ok)
	}
	if serve, ok := MatchRules(rules, EvalContext{"email": "ana@acme.com"}); !ok || !serve {
		t.Errorf("acme user: serve=%v matched=%v", serve, ok)
	}
	if _, ok := MatchRules(rules, nil); ok {
		t.Error("no context must not match")
	}
}

func TestForSDKAppliesRulesBeforeRollout(t *testing.T) {
	r := 0
	flags := []Flag{{Enabled: true, Details: Details{Name: "f"}, Rollout: &r,
		Rules: []Rule{{Attribute: "groups", Operator: "in", Values: []string{"beta"}, Serve: true}}}}
	forSDK(flags, false, EvalContext{"groups": []any{"beta"}})
	if !flags[0].Enabled || flags[0].Rollout != nil {
		t.Errorf("targeted user must get the flag regardless of a 0%% rollout (and old SDKs); got %+v", flags[0])
	}
}

func TestValidateRules(t *testing.T) {
	if err := ValidateRules([]Rule{{Attribute: "email", Operator: "ends_with", Values: []string{"@acme.com"}}}); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
	for _, bad := range [][]Rule{
		{{Attribute: "", Operator: "in", Values: []string{"x"}}},
		{{Attribute: "a", Operator: "regex", Values: []string{"x"}}},
		{{Attribute: "a", Operator: "in"}},
		{{Attribute: "a", Operator: "in", Values: []string{""}}},
	} {
		if ValidateRules(bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestParseContextHeader(t *testing.T) {
	raw := `{"key":"u1","groups":["beta"]}`
	for _, enc := range []string{base64.StdEncoding.EncodeToString([]byte(raw)), base64.RawURLEncoding.EncodeToString([]byte(raw))} {
		ctx, err := ParseContextHeader(enc)
		if err != nil || ctx["key"] != "u1" {
			t.Errorf("%q: ctx=%v err=%v", enc, ctx, err)
		}
	}
	if _, err := ParseContextHeader("not base64!"); err == nil {
		t.Error("garbage accepted")
	}
}
