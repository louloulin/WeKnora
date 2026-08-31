// Package geoiplookup provides the IP→country resolution that lets
// Conditional Access evaluate the country field of a policy without
// the caller having to know how the resolution works.
//
// The package is built around the CountryResolver interface so the
// implementation can be swapped at DI time:
//
//   - NoopCountryResolver: returns ("", false) for every IP. Used by
//     default in environments that have not yet loaded a GeoIP
//     database, or in unit tests that want deterministic answers.
//   - StaticCountryResolver: maps a hand-maintained list of CIDRs to
//     country codes. Useful for small on-prem deployments and for
//     tests that need a known answer for a known IP.
//   - MMDBCountryResolver (optional, behind a build tag in mmdb.go):
//     wraps oschwald/geoip2-golang for production deployments that
//     load a MaxMind GeoLite2-Country.mmdb file from disk.
//
// The resolver returns ISO 3166-1 alpha-2 country codes ("US", "CN",
// "JP", ...) so they line up directly with the conditional_access
// policy's Countries field. Empty / unparseable IPs return
// ("", false); callers must treat that as "country unknown" and
// either skip the country check or apply a deny policy.
//
// The package never errors on lookup failure: a missing database, an
// unmapped IP, or a malformed address all surface as ("", false).
// That matches how Conditional Access treats CountryCode today —
// empty country means "the caller didn't supply one, so country-
// scoped policies don't fire".
package geoiplookup

import (
	"net"
	"strings"
	"sync"
)

// CountryResolver maps an IP address (IPv4 or IPv6) to an ISO 3166-1
// alpha-2 country code. Implementations MUST be safe for concurrent
// use from multiple goroutines: the conditional access evaluator
// calls Lookup from every login request on the hot path.
type CountryResolver interface {
	// Lookup returns the country code for ip. The second return is
	// false when the IP cannot be resolved (unknown IP, no DB loaded,
	// parse error). The first return is uppercase ISO 3166-1 alpha-2.
	Lookup(ip string) (country string, ok bool)
}

// NoopCountryResolver is the safe default. It always reports
// ("", false) so country-scoped policies become inert until a real
// resolver is wired into the DI container.
type NoopCountryResolver struct{}

// Lookup implements CountryResolver.
func (NoopCountryResolver) Lookup(ip string) (string, bool) {
	return "", false
}

// StaticCountryResolver is a hand-maintained CIDR→country map. It is
// intended for on-prem deployments with a small, fixed set of public
// egress IPs and for unit tests that need a deterministic answer.
//
// CIDR matching follows net.ParseCIDR semantics: the input IP must
// match the network portion exactly. The first matching entry wins,
// so more-specific entries should appear earlier in the slice.
type StaticCountryResolver struct {
	mu      sync.RWMutex
	entries []staticEntry
}

type staticEntry struct {
	network *net.IPNet
	country string
}

// NewStaticCountryResolver parses the supplied CIDR→country pairs.
// Malformed entries are dropped silently; callers should validate
// their inputs upstream.
func NewStaticCountryResolver(mapping map[string]string) *StaticCountryResolver {
	entries := make([]staticEntry, 0, len(mapping))
	for cidr, country := range mapping {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		country = strings.ToUpper(strings.TrimSpace(country))
		if country == "" {
			continue
		}
		entries = append(entries, staticEntry{network: network, country: country})
	}
	return &StaticCountryResolver{entries: entries}
}

// Lookup walks the entries in order and returns the first match.
// Returns ("", false) when no entry matches or the input IP is
// unparseable.
func (s *StaticCountryResolver) Lookup(ip string) (string, bool) {
	if s == nil || ip == "" {
		return "", false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if e.network.Contains(parsed) {
			return e.country, true
		}
	}
	return "", false
}

// Add appends a new CIDR→country pair. Useful for the admin handler
// to grow the map at runtime. Safe for concurrent use.
func (s *StaticCountryResolver) Add(cidr, country string) error {
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return err
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return errEmptyCountry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, staticEntry{network: network, country: country})
	return nil
}

// errEmptyCountry is the sentinel returned by Add when the country
// field is empty.
var errEmptyCountry = errStatic("geoiplookup: country must not be empty")

type errStatic string

func (e errStatic) Error() string { return string(e) }
