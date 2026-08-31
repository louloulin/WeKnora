//go:build condaccgeoiptest

package service_test

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/geoiplookup"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestFillCountryFromIP_NoResolver(t *testing.T) {
	req := types.EvaluationRequest{ClientIP: "8.8.8.8"}
	out := service.FillCountryFromIP(req, nil)
	if out.CountryCode != "" {
		t.Fatalf("expected empty country, got %q", out.CountryCode)
	}
}

func TestFillCountryFromIP_PreservesSuppliedCountry(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{"8.8.8.0/24": "US"})
	req := types.EvaluationRequest{ClientIP: "8.8.8.8", CountryCode: "JP"}
	out := service.FillCountryFromIP(req, r)
	if out.CountryCode != "JP" {
		t.Fatalf("expected caller-supplied country to win, got %q", out.CountryCode)
	}
}

func TestFillCountryFromIP_Resolves(t *testing.T) {
	r := geoiplookup.NewStaticCountryResolver(map[string]string{
		"8.8.8.0/24":       "US",
		"114.114.114.0/24": "CN",
	})
	tests := []struct {
		ip       string
		wantCode string
	}{
		{"8.8.8.8", "US"},
		{"114.114.114.114", "CN"},
		{"9.9.9.9", ""},
		{"", ""},
		{"not-an-ip", ""},
	}
	for _, tc := range tests {
		req := types.EvaluationRequest{ClientIP: tc.ip}
		out := service.FillCountryFromIP(req, r)
		if out.CountryCode != tc.wantCode {
			t.Fatalf("Lookup(%q) country = %q, want %q", tc.ip, out.CountryCode, tc.wantCode)
		}
	}
}
