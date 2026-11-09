package odoo

import "strings"

var OdooVersions = []string{
	"19.0",
	"18.0",
	"17.0",
	"16.0",
	"15.0",
	"14.0",
	"13.0",
	"12.0",
}

var DefaultOdooVersion = "19.0"

// VersionsString returns a comma-separated list of supported versions
func VersionsString() string {
	return strings.Join(OdooVersions, ", ")
}
