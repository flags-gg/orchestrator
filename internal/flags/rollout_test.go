package flags

import (
	"strconv"
	"testing"
)

// Shared with every SDK (react-library __tests__/rollout.test.ts): changing them breaks cross-SDK consistency.
var bucketVectors = []struct {
	flag, key string
	want      int
}{
	{"new-checkout", "user-1", 63},
	{"new-checkout", "user-2", 82},
	{"teams", "visitor-abc", 12},
	{"", "", 53},
	{"émoji ✓", "ключ", 95},
}

func TestBucketVectors(t *testing.T) {
	for _, v := range bucketVectors {
		if got := Bucket(v.flag, v.key); got != v.want {
			t.Errorf("Bucket(%q, %q) = %d, want %d", v.flag, v.key, got, v.want)
		}
	}
}

func TestInRollout(t *testing.T) {
	cases := []struct {
		key     string
		rollout int
		want    bool
	}{
		{"user-1", 100, true}, {"", 100, true}, // full rollout ignores the key
		{"user-1", 0, false},
		{"", 50, false},                             // partial rollout without a key fails closed
		{"user-1", 63, false}, {"user-1", 64, true}, // bucket 63: in once rollout > 63
	}
	for _, c := range cases {
		if got := InRollout("new-checkout", c.key, c.rollout); got != c.want {
			t.Errorf("InRollout(%q, %d) = %v, want %v", c.key, c.rollout, got, c.want)
		}
	}

	// roughly the requested share of visitors
	in := 0
	for i := range 10000 {
		if InRollout("new-checkout", "visitor-"+strconv.Itoa(i), 25) {
			in++
		}
	}
	if in < 2200 || in > 2800 {
		t.Errorf("25%% rollout let in %d/10000", in)
	}
}

func TestForSDK(t *testing.T) {
	r := func(n int) *int { return &n }
	mk := func() []Flag {
		return []Flag{
			{Enabled: true, Details: Details{Name: "full"}, Rollout: r(100)},
			{Enabled: true, Details: Details{Name: "part"}, Rollout: r(30)},
			{Enabled: false, Details: Details{Name: "off"}, Rollout: r(30)},
		}
	}

	old := mk()
	forSDK(old, false)
	if !old[0].Enabled || old[1].Enabled || old[0].Rollout != nil || old[1].Rollout != nil {
		t.Errorf("old SDK: want full on, partial off, no rollout field; got %+v", old)
	}

	aware := mk()
	forSDK(aware, true)
	if aware[0].Rollout != nil || !aware[1].Enabled || aware[1].Rollout == nil || *aware[1].Rollout != 30 {
		t.Errorf("rollout-aware SDK: want partial kept with rollout 30; got %+v", aware)
	}
}

func TestEvaluateSplit(t *testing.T) {
	r := 64
	f := Flag{Enabled: true, Details: Details{Name: "new-checkout", ID: "1"}, Rollout: &r}
	if res := evaluate("new-checkout", f, "user-1"); !res.Value.(bool) || res.Reason != ReasonSplit {
		t.Errorf("user-1 (bucket 63) at 64%%: got %v %s", res.Value, res.Reason)
	}
	if res := evaluate("new-checkout", f, "user-2"); res.Value.(bool) || res.Variant != "disabled" {
		t.Errorf("user-2 (bucket 82) at 64%%: got %v", res.Value)
	}
	f.Enabled = false
	if res := evaluate("new-checkout", f, "user-1"); res.Value.(bool) || res.Reason != ReasonStatic {
		t.Errorf("disabled flag must stay off and STATIC; got %v %s", res.Value, res.Reason)
	}
}
