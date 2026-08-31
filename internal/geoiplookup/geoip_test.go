package geoiplookup_test

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/geoiplookup"
)

func TestNoopCountryResolver_AlwaysMisses(t *testing.T) {
	r := geoiplookup.NoopCountryResolver{}
	cases := []string{"", "0.0.0.0", "8.8.8.8", "::1", "not-an-ip"}
	for _, ip := range cases {
		country, ok := r.Lookup(ip)
		if ok || country != "" {
			t.Fatalf("expected miss for %q, got (%q,%v)", ip, country, ok)
		}
	}
}

func TestStaticCountryResolver_BasicMatch(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{
		"8.8.8.0/24":       "US",
		"114.114.114.0/24": "CN",
		"1.1.1.0/24":       "US",
	})
	tests := []struct {
		ip       string
		wantOK   bool
		wantCtry string
	}{
		{"8.8.8.8", true, "US"},
		{"8.8.4.4", false, ""}, // outside the 8.8.8.0/24
		{"114.114.114.114", true, "CN"},
		{"114.114.115.114", false, ""},
		{"1.1.1.1", true, "US"},
		{"9.9.9.9", false, ""},
		{"", false, ""},
		{"not-an-ip", false, ""},
	}
	for _, tc := range tests {
		gotCtry, gotOK := r.Lookup(tc.ip)
		if gotOK != tc.wantOK || gotCtry != tc.wantCtry {
			t.Fatalf("Lookup(%q) = (%q,%v), want (%q,%v)",
				tc.ip, gotCtry, gotOK, tc.wantCtry, tc.wantOK)
		}
	}
}

func TestStaticCountryResolver_CountryIsUppercased(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{
		"10.0.0.0/8": "us",
	})
	if ctry, ok := r.Lookup("10.1.2.3"); !ok || ctry != "US" {
		t.Fatalf("expected US, got (%q,%v)", ctry, ok)
	}
}

func TestStaticCountryResolver_DropsInvalid(t *testing.T) {
	// Disjoint ranges so dropped entries really miss.
	r := geoiplookup.NewStaticCountryResolver(map[string]string{
		"bogus":       "US",
		"10.0.0.0/16": "us",
		"10.1.0.0/16": "",  // empty country → dropped
		"10.2.0.0/16": " ", // whitespace-only → dropped
	})
	if ctry, ok := r.Lookup("10.0.0.1"); !ok || ctry != "US" {
		t.Fatalf("expected US, got (%q,%v)", ctry, ok)
	}
	if _, ok := r.Lookup("10.1.0.1"); ok {
		t.Fatalf("expected miss for empty-country CIDR")
	}
	if _, ok := r.Lookup("10.2.0.1"); ok {
		t.Fatalf("expected miss for whitespace-only country")
	}
}

func TestStaticCountryResolver_Add(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{})
	if _, ok := r.Lookup("192.30.255.112"); ok {
		t.Fatalf("expected miss on empty resolver")
	}
	if err := r.Add("192.30.255.0/24", "US"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ctry, ok := r.Lookup("192.30.255.112"); !ok || ctry != "US" {
		t.Fatalf("expected US, got (%q,%v)", ctry, ok)
	}
	if err := r.Add("not-a-cidr", "US"); err == nil {
		t.Fatalf("expected error on bad CIDR")
	}
	if err := r.Add("10.0.0.0/8", ""); err == nil {
		t.Fatalf("expected error on empty country")
	}
}

func TestStaticCountryResolver_IPv6(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{
		"2001:db8::/32": "XX",
	})
	if ctry, ok := r.Lookup("2001:db8::1"); !ok || ctry != "XX" {
		t.Fatalf("expected XX for IPv6, got (%q,%v)", ctry, ok)
	}
	if _, ok := r.Lookup("2001:db9::1"); ok {
		t.Fatalf("expected miss for non-matching IPv6")
	}
}

// Compile-time check that NoopCountryResolver satisfies CountryResolver.
var _ geoiplookup.CountryResolver = geoiplookup.NoopCountryResolver{}
