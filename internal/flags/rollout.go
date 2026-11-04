package flags

import "hash/fnv"

// RolloutCapability is the x-flags-capabilities value an SDK sends when it evaluates rollouts itself.
const RolloutCapability = "rollout"

// Bucket places a visitor in 0..99 for a flag. Every SDK must compute exactly this:
// FNV-1a 32-bit of "<flag name>:<targeting key>", modulo 100.
func Bucket(flagName, targetingKey string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flagName + ":" + targetingKey))
	return int(h.Sum32() % 100)
}

// InRollout reports whether the visitor sees an enabled flag with this rollout percentage.
// Without a targeting key only a full rollout applies, so partial rollouts fail closed.
func InRollout(flagName, targetingKey string, rollout int) bool {
	switch {
	case rollout >= 100:
		return true
	case rollout <= 0 || targetingKey == "":
		return false
	}
	return Bucket(flagName, targetingKey) < rollout
}

// forSDK shapes flags for an SDK. Targeting rules are decided here, against the context the SDK sent,
// and never leave the server. Otherwise full rollouts omit the field, partial ones are sent to SDKs that
// can evaluate them and are sent disabled to older SDKs, which would otherwise show them to everyone.
func forSDK(flags []Flag, rolloutAware bool, ctx EvalContext) {
	for i := range flags {
		f := &flags[i]
		if serve, matched := MatchRules(f.Rules, ctx); f.Enabled && matched {
			f.Enabled, f.Rollout = serve, nil
			continue
		}
		if f.Rollout == nil || *f.Rollout >= 100 {
			f.Rollout = nil
			continue
		}
		if !rolloutAware {
			f.Enabled = false
			f.Rollout = nil
		}
	}
}
