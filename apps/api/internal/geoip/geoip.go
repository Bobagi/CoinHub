// Package geoip resolves an IP address to a coarse location (country/region/city) using a local
// MaxMind GeoLite2-City database. The lookup is offline — the IP never leaves the server — matching
// how analytics tools (e.g. Umami) geolocate. It degrades gracefully: with no database configured the
// Locator is a no-op and every lookup returns an empty Location.
package geoip

import (
	"log"
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// Location is the coarse place an access came from. Any field may be empty when unknown.
type Location struct {
	CountryCode string
	CountryName string
	Region      string
	City        string
}

// Locator reads a GeoLite2-City database. A nil Locator (or one opened without a database) is valid
// and behaves as a no-op, so callers never need to nil-check.
type Locator struct {
	reader *geoip2.Reader
}

// Open opens a GeoLite2-City database at databasePath. An empty path or an open failure yields a
// no-op Locator (logged once) rather than an error, so geolocation is strictly optional.
func Open(databasePath string) *Locator {
	if strings.TrimSpace(databasePath) == "" {
		log.Printf("geoip: no city database configured (GEOIP_CITY_DB unset) — geolocation disabled")
		return &Locator{}
	}
	reader, openError := geoip2.Open(databasePath)
	if openError != nil {
		log.Printf("geoip: could not open city database %q (geolocation disabled): %v", databasePath, openError)
		return &Locator{}
	}
	log.Printf("geoip: city database loaded from %s", databasePath)
	return &Locator{reader: reader}
}

// Enabled reports whether a real database is loaded.
func (locator *Locator) Enabled() bool {
	return locator != nil && locator.reader != nil
}

// Close releases the database, if any.
func (locator *Locator) Close() {
	if locator != nil && locator.reader != nil {
		_ = locator.reader.Close()
	}
}

// Lookup resolves an IP to a coarse Location. languageKey selects localized place names (e.g. "en",
// "pt-BR", "es"); it falls back to English then to whatever is available. Unknown/invalid IPs and a
// disabled Locator both return an empty Location.
func (locator *Locator) Lookup(ipAddress string, languageKey string) Location {
	if !locator.Enabled() {
		return Location{}
	}
	parsedIP := net.ParseIP(strings.TrimSpace(ipAddress))
	if parsedIP == nil {
		return Location{}
	}
	record, lookupError := locator.reader.City(parsedIP)
	if lookupError != nil || record == nil {
		return Location{}
	}
	location := Location{
		CountryCode: record.Country.IsoCode,
		CountryName: localizedName(record.Country.Names, languageKey),
		City:        localizedName(record.City.Names, languageKey),
	}
	if len(record.Subdivisions) > 0 {
		location.Region = localizedName(record.Subdivisions[0].Names, languageKey)
	}
	return location
}

// LanguageKey maps an app locale (en/pt/es) to the GeoLite2 name key used for localized place names.
func LanguageKey(locale string) string {
	switch locale {
	case "pt":
		return "pt-BR"
	case "es":
		return "es"
	default:
		return "en"
	}
}

func localizedName(names map[string]string, languageKey string) string {
	if name, found := names[languageKey]; found && name != "" {
		return name
	}
	if name, found := names["en"]; found {
		return name
	}
	return ""
}
