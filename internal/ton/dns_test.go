package ton

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foundation.ton", "foundation.ton"},
		{"http://foundation.ton", "foundation.ton"},
		{"https://foundation.ton", "foundation.ton"},
		{"  foundation.ton  ", "foundation.ton"},
		{"http://foundation.ton/path", "foundation.ton"},
		{"http://sub.foundation.ton/path?q=1", "sub.foundation.ton"},
	}
	for _, tc := range tests {
		got := normalizeDomain(tc.input)
		if got != tc.want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveRecord_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before calling

	_, err := ResolveRecord(ctx, "foundation.ton")
	if err == nil {
		t.Fatal("ResolveRecord with canceled context: expected error, got nil")
	}
}

func TestResolveRecord_MockServer_FullRecord(t *testing.T) {
	mock := tonAPIResolveResponse{
		Wallet:  &tonAPIWallet{Address: "0:abc123def456", Name: "TON Foundation"},
		Sites:   []string{"adnlhex123"},
		Storage: "bagid456",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mock) //nolint:errcheck
	}))
	defer srv.Close()

	orig := tonAPIBase
	tonAPIBase = srv.URL
	defer func() { tonAPIBase = orig }()

	rec, err := ResolveRecord(context.Background(), "foundation.ton")
	if err != nil {
		t.Fatalf("ResolveRecord: unexpected error: %v", err)
	}
	if rec.Domain != "foundation.ton" {
		t.Errorf("Domain = %q, want %q", rec.Domain, "foundation.ton")
	}
	if rec.Wallet != "0:abc123def456" {
		t.Errorf("Wallet = %q, want %q", rec.Wallet, "0:abc123def456")
	}
	if rec.ADNL != "adnlhex123" {
		t.Errorf("ADNL = %q, want %q", rec.ADNL, "adnlhex123")
	}
	if rec.Storage != "bagid456" {
		t.Errorf("Storage = %q, want %q", rec.Storage, "bagid456")
	}
	if !rec.Resolved {
		t.Error("Resolved = false, want true")
	}
}

func TestResolveRecord_MockServer_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := tonAPIBase
	tonAPIBase = srv.URL
	defer func() { tonAPIBase = orig }()

	rec, err := ResolveRecord(context.Background(), "unknown.ton")
	if err != nil {
		t.Fatalf("ResolveRecord 404: unexpected error: %v", err)
	}
	if rec.Resolved {
		t.Error("Resolved = true for 404 response, want false")
	}
	if rec.Domain != "unknown.ton" {
		t.Errorf("Domain = %q, want %q", rec.Domain, "unknown.ton")
	}
}

func TestResolveRecord_MockServer_EmptySites(t *testing.T) {
	mock := tonAPIResolveResponse{
		Wallet: &tonAPIWallet{Address: "0:walletonly"},
		Sites:  []string{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mock) //nolint:errcheck
	}))
	defer srv.Close()

	orig := tonAPIBase
	tonAPIBase = srv.URL
	defer func() { tonAPIBase = orig }()

	rec, err := ResolveRecord(context.Background(), "wallet.ton")
	if err != nil {
		t.Fatalf("ResolveRecord: unexpected error: %v", err)
	}
	if rec.ADNL != "" {
		t.Errorf("ADNL = %q for empty sites, want empty string", rec.ADNL)
	}
	if !rec.Resolved {
		t.Error("Resolved = false, want true (wallet is set)")
	}
}

func TestResolveRecord_MockServer_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := tonAPIBase
	tonAPIBase = srv.URL
	defer func() { tonAPIBase = orig }()

	_, err := ResolveRecord(context.Background(), "error.ton")
	if err == nil {
		t.Fatal("ResolveRecord 500: expected error, got nil")
	}
}

func TestResolveRecord_DomainNormalizationInURL(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tonAPIResolveResponse{}) //nolint:errcheck
	}))
	defer srv.Close()

	orig := tonAPIBase
	tonAPIBase = srv.URL
	defer func() { tonAPIBase = orig }()

	ResolveRecord(context.Background(), "http://foundation.ton") //nolint:errcheck

	want := "/dns/foundation.ton/resolve"
	if capturedPath != want {
		t.Errorf("URL path = %q, want %q", capturedPath, want)
	}
}
