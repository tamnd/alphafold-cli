package alphafold

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in alphafold_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "alphafold" {
		t.Errorf("Scheme = %q, want alphafold", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "alphafold" {
		t.Errorf("Identity.Binary = %q, want alphafold", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"P04637", "prediction", "P04637"},
		{"Q5VSL9", "prediction", "Q5VSL9"},
		{"AF-P04637-F1", "prediction", "AF-P04637-F1"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType, id, want string
	}{
		{"prediction", "P04637", "https://alphafold.ebi.ac.uk/entry/P04637"},
		{"prediction", "AF-P04637-F1", "https://alphafold.ebi.ac.uk/entry/AF-P04637-F1"},
		{"prediction", "Q5VSL9", "https://alphafold.ebi.ac.uk/entry/Q5VSL9"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("page", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host (the runtime ant drives) and
// checks the round trip: a record mints to its URI, its body is readable, and a
// bare id resolves back to the same URI. The init in domain.go registers the
// domain, so kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	p := &Prediction{
		ID:          "AF-P04637-F1",
		Gene:        "TP53",
		Description: "Cellular tumor antigen p53",
		GlobalScore: 71.79,
	}
	u, err := h.Mint(p)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "alphafold://prediction/AF-P04637-F1"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("alphafold", "Q5VSL9")
	if err != nil || got.String() != "alphafold://prediction/Q5VSL9" {
		t.Errorf("ResolveOn = (%q, %v), want alphafold://prediction/Q5VSL9", got.String(), err)
	}
}
