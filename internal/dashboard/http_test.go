package dashboard

import "testing"

func TestNewerId(t *testing.T) {
	if !newerId("1234", "99") || newerId("99", "1234") {
		t.Fatal("ids must compare numerically")
	}
}

func TestParseTimestamp(t *testing.T) {
	for _, v := range []string{"2025-12-25 14:53:00.123456", "2025-12-25 14:53:00", "2025-12-25T14:53:00Z"} {
		if parseTimestamp(v) == 0 {
			t.Fatalf("failed to parse %q", v)
		}
	}
}
