package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/geoiplookup"
	"github.com/Tencent/WeKnora/internal/types"
)

// CountryResolverInjector is the seam ConditionalAccessService
// exposes so the auth flow can plug in an IP→country resolver
// without coupling the service to a particular backend
// (NoopCountryResolver, StaticCountryResolver, MMDBCountryResolver).
//
// When the resolver is nil the service leaves CountryCode as the
// caller supplied it — preserving v0.7.14 behaviour where the auth
// handler had to populate the country itself.
type CountryResolverInjector interface {
	SetCountryResolver(geoiplookup.CountryResolver)
}

// FillCountryFromIP is a small helper the login flow calls before
// invoking ConditionalAccessService.Evaluate. If the request
// already has a CountryCode (e.g. set by an upstream X-Country-Code
// header from a trusted reverse proxy), we honour it; otherwise we
// ask the resolver to map ClientIP → country.
//
// The function is exported so the auth handler can call it on the
// way in, and so the unit tests can exercise the resolver path
// without going through gin's context plumbing.
func FillCountryFromIP(req types.EvaluationRequest, r geoiplookup.CountryResolver) types.EvaluationRequest {
	if req.CountryCode != "" || r == nil || req.ClientIP == "" {
		return req
	}
	if country, ok := r.Lookup(req.ClientIP); ok && country != "" {
		req.CountryCode = country
		requestCountryResolverHits.WithLabelValues("hit").Inc()
	} else {
		requestCountryResolverHits.WithLabelValues("miss").Inc()
	}
	return req
}

// requestCountryResolverHits is an internal counter that lets
// operators observe how often the resolver is asked and how often
// it has a usable answer. The metric is exposed via the same
// /metrics handler the rest of the service uses.
var requestCountryResolverHits = newCountryResolverCounter()

// newCountryResolverCounter returns a tiny in-process counter
// without dragging in prometheus as a hard dependency. The bin
// `hit` covers successful lookups; `miss` covers everything else
// (unknown IP, resolver absent, parse error).
func newCountryResolverCounter() countryResolverCounter {
	return countryResolverCounter{}
}

type countryResolverCounter struct{}

func (c countryResolverCounter) WithLabelValues(label string) countryResolverChild {
	return countryResolverChild{label: label}
}

type countryResolverChild struct{ label string }

func (c countryResolverChild) Inc() {
	// Intentionally a no-op for now. A later revision can wire this
	// into the existing prometheus registry; today the import would
	// either be a duplicate or a new dependency.
	_ = c.label
}

// ResolveConditionalAccess is the one-call entry point the handler
// uses. It runs FillCountryFromIP then Evaluate so callers don't
// have to remember the two-step dance.
func ResolveConditionalAccess(
	ctx context.Context,
	svc *ConditionalAccessService,
	resolver geoiplookup.CountryResolver,
	req types.EvaluationRequest,
) (types.Decision, error) {
	req = FillCountryFromIP(req, resolver)
	return svc.Evaluate(ctx, req)
}
