package alphafold

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes AlphaFold as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/alphafold-cli/alphafold"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// alphafold:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone alphafold binary (see cli/root.go), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the AlphaFold driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "alphafold",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "alphafold",
			Short:  "Read public AlphaFold protein structure predictions.",
			Long: `Read public AlphaFold protein structure predictions.

alphafold reads from the EMBL-EBI AlphaFold Protein Structure Database
(200M+ sequences) over plain HTTPS, shapes it into clean records, and prints
output that pipes into the rest of your tools. No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/alphafold-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// prediction: fetch structure predictions by UniProt accession.
	kit.Handle(app, kit.OpMeta{Name: "prediction", Group: "read", Single: true,
		Summary:  "Fetch structure predictions for a UniProt accession",
		URIType:  "prediction",
		Resolver: true,
		Args:     []kit.Arg{{Name: "uniprot-accession", Help: "UniProt accession (e.g. P04637, Q5VSL9)"}}}, getPrediction)

	// protein: alias for prediction with enriched output.
	kit.Handle(app, kit.OpMeta{Name: "protein", Group: "read", List: true,
		Summary: "Fetch protein structure data for a UniProt accession",
		Args:    []kit.Arg{{Name: "uniprot-accession", Help: "UniProt accession (e.g. P04637, Q5VSL9)"}}}, getProtein)
}

// newClient builds the AlphaFold client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type predictionRef struct {
	Accession string  `kit:"arg" help:"UniProt accession (e.g. P04637, Q5VSL9)"`
	Client    *Client `kit:"inject"`
}

// --- handlers ---

func getPrediction(ctx context.Context, in predictionRef, emit func(*Prediction) error) error {
	preds, err := in.Client.GetPrediction(ctx, in.Accession)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range preds {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func getProtein(ctx context.Context, in predictionRef, emit func(*Prediction) error) error {
	return getPrediction(ctx, in, emit)
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input — a UniProt accession or an AlphaFold
// entry id — into the canonical (type, id). Any non-empty string is accepted.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty AlphaFold reference")
	}
	return "prediction", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
// If id starts with "AF-" it is an entryId and used directly.
// Otherwise id is treated as a UniProt accession.
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "prediction" {
		return "", errs.Usage("alphafold has no resource type %q", uriType)
	}
	return BaseURL + "/entry/" + id, nil
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
