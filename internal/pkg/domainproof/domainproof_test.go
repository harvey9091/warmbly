package domainproof

import (
	"testing"

	"github.com/google/uuid"
)

func TestValueIsBoundToWorkspaceDomainAndSecret(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	p := New("instance-secret-one")
	v := p.Value(a, "Acme.io.")
	if v != p.Value(a, "acme.io") {
		t.Fatal("case or trailing dot changed the value")
	}
	if v == p.Value(b, "acme.io") || v == p.Value(a, "other.io") || v == New("instance-secret-two").Value(a, "acme.io") {
		t.Fatal("the value is not bound to workspace, domain and secret")
	}
	if !p.Matches([]string{"v=spf1 -all", " " + v + " "}, a, "acme.io") || p.Matches([]string{v}, b, "acme.io") {
		t.Fatal("Matches accepted the wrong workspace or missed the right one")
	}
	if Name("Acme.io") != "_warmbly.acme.io" {
		t.Fatal(Name("Acme.io"))
	}
}
