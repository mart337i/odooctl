package browser

import (
	"testing"

	"github.com/mart337i/odooctl/internal/config"
)

func TestResolveURLsForPath(t *testing.T) {
	state := &config.State{Ports: config.Ports{Odoo: 9900}}
	public, internal, err := resolveURLs(state, "/web")
	if err != nil {
		t.Fatal(err)
	}
	if public != "http://localhost:9900/web" || internal != "http://127.0.0.1:8069/web" {
		t.Fatalf("public=%q internal=%q", public, internal)
	}
}

func TestResolveURLsRewritesLocalhostPublicURL(t *testing.T) {
	state := &config.State{Ports: config.Ports{Odoo: 9900}}
	public, internal, err := resolveURLs(state, "http://localhost:9900/web?debug=1")
	if err != nil {
		t.Fatal(err)
	}
	if public != "http://localhost:9900/web?debug=1" || internal != "http://127.0.0.1:8069/web?debug=1" {
		t.Fatalf("public=%q internal=%q", public, internal)
	}
}
