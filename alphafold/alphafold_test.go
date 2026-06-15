package alphafold

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetAcceptHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	_, _ = c.Get(context.Background(), srv.URL)
}

func TestGetPrediction(t *testing.T) {
	resp := []wirePrediction{
		{
			EntryID:            "AF-P04637-F1",
			UniprotAccession:   "P04637",
			UniprotID:          "P53_HUMAN",
			UniprotDescription: "Cellular tumor antigen p53",
			TaxID:              9606,
			OrganismName:       "Homo sapiens",
			Gene:               "TP53",
			ModelCreatedDate:   "2022-06-01",
			LatestVersion:      4,
			IsReviewed:         true,
			IsReferenceProteome: true,
			Sequence:           "MEEPQSDPSVEPPLSQ",
			PdbURL:             "https://alphafold.ebi.ac.uk/files/AF-P04637-F1-model_v4.pdb",
			CifURL:             "https://alphafold.ebi.ac.uk/files/AF-P04637-F1-model_v4.cif",
			GlobalMetric:       71.79,
			PlddtVeryHigh:      0.38,
			PlddtConfident:     0.30,
			PlddtLow:           0.22,
			PlddtVeryLow:       0.10,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/prediction/P04637" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	preds, err := c.getPredictionURL(context.Background(), srv.URL+"/api/prediction/P04637")
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 1 {
		t.Fatalf("len(preds) = %d, want 1", len(preds))
	}
	p := preds[0]
	if p.ID != "AF-P04637-F1" {
		t.Errorf("ID = %q, want AF-P04637-F1", p.ID)
	}
	if p.Gene != "TP53" {
		t.Errorf("Gene = %q, want TP53", p.Gene)
	}
	if p.Organism != "Homo sapiens" {
		t.Errorf("Organism = %q, want Homo sapiens", p.Organism)
	}
	if p.TaxID != 9606 {
		t.Errorf("TaxID = %d, want 9606", p.TaxID)
	}
	if p.Version != 4 {
		t.Errorf("Version = %d, want 4", p.Version)
	}
	if p.GlobalScore != 71.79 {
		t.Errorf("GlobalScore = %f, want 71.79", p.GlobalScore)
	}
	if p.SequenceLen != len("MEEPQSDPSVEPPLSQ") {
		t.Errorf("SequenceLen = %d, want %d", p.SequenceLen, len("MEEPQSDPSVEPPLSQ"))
	}
	if p.PdbURL != "https://alphafold.ebi.ac.uk/files/AF-P04637-F1-model_v4.pdb" {
		t.Errorf("PdbURL = %q", p.PdbURL)
	}
}

func TestGetPredictionMultiple(t *testing.T) {
	resp := []wirePrediction{
		{EntryID: "AF-Q5VSL9-F1", UniprotAccession: "Q5VSL9", Gene: "BRCA2", Sequence: "MPIGSKERP"},
		{EntryID: "AF-Q5VSL9-F2", UniprotAccession: "Q5VSL9", Gene: "BRCA2", Sequence: "ACDEFGHIKL"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	preds, err := c.getPredictionURL(context.Background(), srv.URL+"/api/prediction/Q5VSL9")
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 2 {
		t.Fatalf("len(preds) = %d, want 2", len(preds))
	}
	if preds[0].ID != "AF-Q5VSL9-F1" {
		t.Errorf("preds[0].ID = %q, want AF-Q5VSL9-F1", preds[0].ID)
	}
	if preds[1].ID != "AF-Q5VSL9-F2" {
		t.Errorf("preds[1].ID = %q, want AF-Q5VSL9-F2", preds[1].ID)
	}
	if preds[0].SequenceLen != 9 {
		t.Errorf("preds[0].SequenceLen = %d, want 9", preds[0].SequenceLen)
	}
}
