package ton

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// tonAPIBase is the base URL for TonAPI v2. Overridable in tests.
var tonAPIBase = "https://tonapi.io/v2"

// DNSRecord holds the resolution result for a .ton domain.
type DNSRecord struct {
	Domain   string `json:"domain"`
	Wallet   string `json:"wallet,omitempty"`
	ADNL     string `json:"adnl,omitempty"`
	Storage  string `json:"storage,omitempty"`
	NFT      string `json:"nft,omitempty"`
	Resolved bool   `json:"resolved"`
}

// tonAPIResolveResponse is the raw JSON shape returned by TonAPI v2 /dns/{domain}/resolve.
type tonAPIResolveResponse struct {
	Wallet       *tonAPIWallet `json:"wallet"`
	Sites        []string      `json:"sites"`
	Storage      string        `json:"storage"`
	NextResolver string        `json:"next_resolver"`
}

type tonAPIWallet struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// normalizeDomain strips scheme prefix, trailing path, and spaces from a domain string.
func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if idx := strings.Index(domain, "://"); idx != -1 {
		domain = domain[idx+3:]
	}
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	return domain
}

// ResolveRecord queries TonAPI v2 to resolve a .ton domain.
// Returns wallet address, ADNL site address, and storage bag ID.
// API: GET https://tonapi.io/v2/dns/{domain}/resolve
func ResolveRecord(ctx context.Context, domain string) (*DNSRecord, error) {
	domain = normalizeDomain(domain)

	url := fmt.Sprintf("%s/dns/%s/resolve", tonAPIBase, domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("dns: create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dns: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &DNSRecord{Domain: domain, Resolved: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dns: unexpected status %d for domain %q", resp.StatusCode, domain)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dns: read response: %w", err)
	}

	var raw tonAPIResolveResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("dns: parse response: %w", err)
	}

	rec := &DNSRecord{Domain: domain}
	if raw.Wallet != nil {
		rec.Wallet = raw.Wallet.Address
	}
	if len(raw.Sites) > 0 {
		rec.ADNL = raw.Sites[0]
	}
	rec.Storage = raw.Storage
	rec.Resolved = rec.Wallet != "" || rec.ADNL != "" || rec.Storage != ""

	return rec, nil
}
