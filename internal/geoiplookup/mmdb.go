//go:build !geoipmmdb

package geoiplookup

// MMDBResolver is the production-grade IP→country resolver backed by
// a MaxMind GeoLite2-Country.mmdb file. The full implementation
// requires github.com/oschwald/geoip2-golang and is gated behind the
// `geoipmmdb` build tag so the base binary stays CGO-free.
//
// To enable it in production:
//
//	go install --tags geoipmmdb
//	# then point GEOIP_MMDB_PATH at the .mmdb file in the runtime env
//
// When the tag is absent (the default build), the call to NewMMDBCountryResolver
// returns an error at startup; the rest of the system falls back to
// the NoopCountryResolver so deployments without an mmdb file keep
// running.
func NewMMDBCountryResolver(path string) (CountryResolver, error) {
	_ = path
	return nil, errMMDBSupportDisabled
}

var errMMDBSupportDisabled = errStatic("geoiplookup: mmdb support compiled out (rebuild with -tags geoipmmdb)")
