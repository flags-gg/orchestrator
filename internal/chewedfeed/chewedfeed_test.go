package chewedfeed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendUsesTheStandardHook(t *testing.T) {
	var got customer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/product-customers/sync" || r.Header.Get("X-Enquiry-Key") != "ek" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()

	if err := send(context.Background(), srv.URL, "ek", customer{ExternalCustomerID: "c1", Email: "a@b.c", Plan: "startup"}); err != nil {
		t.Fatal(err)
	}
	if got.ExternalCustomerID != "c1" || got.Plan != "startup" {
		t.Fatalf("sent %+v", got)
	}
	if err := send(context.Background(), srv.URL, "wrong", customer{}); err == nil {
		t.Fatal("a refused sync must be an error")
	}
}
