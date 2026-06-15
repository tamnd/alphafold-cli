// Package alphafold is the library behind the alphafold command line:
// the HTTP client, request shaping, and the typed data models for
// the EMBL-EBI AlphaFold Protein Structure Database.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package alphafold

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to AlphaFold.
const DefaultUserAgent = "alphafold-cli/dev (+https://github.com/tamnd/alphafold-cli)"

// Host is the AlphaFold site this client talks to.
const Host = "alphafold.ebi.ac.uk"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to AlphaFold over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 300ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches rawURL and returns the response body. It paces and retries
// according to the client's settings. The body is read fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (unexported) ---

type wirePrediction struct {
	EntryID             string  `json:"entryId"`
	UniprotAccession    string  `json:"uniprotAccession"`
	UniprotID           string  `json:"uniprotId"`
	UniprotDescription  string  `json:"uniprotDescription"`
	TaxID               int     `json:"taxId"`
	OrganismName        string  `json:"organismScientificName"`
	Gene                string  `json:"gene"`
	ModelCreatedDate    string  `json:"modelCreatedDate"`
	LatestVersion       int     `json:"latestVersion"`
	IsReviewed          bool    `json:"isReviewed"`
	IsReferenceProteome bool    `json:"isReferenceProteome"`
	Sequence            string  `json:"sequence"`
	PdbURL              string  `json:"pdbUrl"`
	CifURL              string  `json:"cifUrl"`
	GlobalMetric        float64 `json:"globalMetricValue"`
	PlddtVeryHigh       float64 `json:"fractionPlddtVeryHigh"`
	PlddtConfident      float64 `json:"fractionPlddtConfident"`
	PlddtLow            float64 `json:"fractionPlddtLow"`
	PlddtVeryLow        float64 `json:"fractionPlddtVeryLow"`
}

// --- public output types ---

// Prediction is a single AlphaFold predicted protein structure record.
type Prediction struct {
	ID          string  `json:"id"                   kit:"id"`
	UniProtID   string  `json:"uniprot_id,omitempty"`
	Description string  `json:"description,omitempty"`
	Gene        string  `json:"gene,omitempty"`
	Organism    string  `json:"organism,omitempty"`
	TaxID       int     `json:"tax_id,omitempty"`
	Version     int     `json:"version,omitempty"`
	ModelDate   string  `json:"model_date,omitempty"`
	IsReviewed  bool    `json:"is_reviewed,omitempty"`
	GlobalScore float64 `json:"plddt_score,omitempty"`
	PdbURL      string  `json:"pdb_url,omitempty"`
	SequenceLen int     `json:"sequence_length,omitempty"`
}

// --- client methods ---

// GetPrediction fetches all structure prediction fragments for a UniProt
// accession (e.g. "P04637"). Returns one Prediction per fragment entry.
func (c *Client) GetPrediction(ctx context.Context, uniprotAccession string) ([]*Prediction, error) {
	rawURL := BaseURL + "/api/prediction/" + uniprotAccession
	return c.getPredictionURL(ctx, rawURL)
}

// getPredictionURL is the testable core of GetPrediction; tests point it at
// an httptest server without touching BaseURL.
func (c *Client) getPredictionURL(ctx context.Context, rawURL string) ([]*Prediction, error) {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var ws []wirePrediction
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("prediction parse: %w", err)
	}
	out := make([]*Prediction, 0, len(ws))
	for _, w := range ws {
		out = append(out, predictionFromWire(w))
	}
	return out, nil
}

// --- helpers ---

// predictionFromWire converts a wire prediction to the public Prediction type.
func predictionFromWire(w wirePrediction) *Prediction {
	return &Prediction{
		ID:          w.EntryID,
		UniProtID:   w.UniprotID,
		Description: w.UniprotDescription,
		Gene:        w.Gene,
		Organism:    w.OrganismName,
		TaxID:       w.TaxID,
		Version:     w.LatestVersion,
		ModelDate:   w.ModelCreatedDate,
		IsReviewed:  w.IsReviewed,
		GlobalScore: w.GlobalMetric,
		PdbURL:      w.PdbURL,
		SequenceLen: len(w.Sequence),
	}
}
