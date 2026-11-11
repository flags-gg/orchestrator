package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestSignature(t *testing.T) {
	body := []byte(`{"id":"event-1","enabled":true}`)
	mac := hmac.New(sha256.New, []byte("whsec_test"))
	mac.Write([]byte("1700000000."))
	mac.Write(body)
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if got := Signature("whsec_test", "1700000000", body); got != want {
		t.Fatalf("%s != %s", got, want)
	}
	if Signature("whsec_test", "1700000000", append(body, ' ')) == want {
		t.Fatal("body changes must invalidate signature")
	}
	if Signature("whsec_test", "1700000001", body) == want {
		t.Fatal("timestamp must be signed")
	}
}
func TestPublicDestinations(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.1.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "0.0.0.0", "192.0.2.1", "2001:db8::1", "64:ff9b::7f00:1", "2002:7f00:1::1"} {
		if publicIP(netip.MustParseAddr(ip)) {
			t.Errorf("accepted %s", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(ip)) {
			t.Errorf("rejected %s", ip)
		}
	}
	for _, raw := range []string{"http://example.com", "https://user:password@example.com", "https://example.com:8080", "https://example.com/#fragment", "https://127.0.0.1", "https://[::1]", "https://"} {
		if _, err := validURL(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com/hook", "https://example.com:443/hook?key=x"} {
		if _, err := validURL(raw); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
}
func TestRetryScheduleAndSubscriptions(t *testing.T) {
	for attempt, want := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour, 12 * time.Hour, 24 * time.Hour} {
		if retryDelay(attempt+1) != want {
			t.Fatalf("attempt %d", attempt+1)
		}
	}
	for _, events := range [][]string{nil, {}, {"flag.created", "flag.created"}, {"unknown"}} {
		if validEvents(events) {
			t.Fatal(events)
		}
	}
	if !validEvents([]string{"flag.created", "flag.archived"}) {
		t.Fatal("valid events rejected")
	}
	if !strings.HasPrefix(Signature("s", "t", nil), "v1=") {
		t.Fatal("signature version missing")
	}
}
