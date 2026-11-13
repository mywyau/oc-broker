package httpapi

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"known":"value","surprise":true}`))
	w := httptest.NewRecorder()
	var dst struct {
		Known string `json:"known"`
	}
	if err := decodeJSON(w, req, &dst); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestParseClaimantListQueryDefaults(t *testing.T) {
	query, err := parseClaimantListQuery(url.Values{})
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if query.Limit != 20 || query.Offset != 0 || query.Search != "" {
		t.Fatalf("unexpected defaults: %+v", query)
	}
}

func TestParseClaimantListQuery(t *testing.T) {
	query, err := parseClaimantListQuery(url.Values{"search": {"  Alex  "}, "limit": {"25"}, "offset": {"50"}})
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if query.Search != "Alex" || query.Limit != 25 || query.Offset != 50 {
		t.Fatalf("unexpected query: %+v", query)
	}
}

func TestParseClaimantListQueryRejectsInvalidPagination(t *testing.T) {
	tests := []url.Values{
		{"limit": {"0"}},
		{"limit": {"101"}},
		{"limit": {"many"}},
		{"offset": {"-1"}},
		{"offset": {"next"}},
	}
	for _, values := range tests {
		if _, err := parseClaimantListQuery(values); err == nil {
			t.Fatalf("expected query to be rejected: %v", values)
		}
	}
}

func TestDecodeJSONAcceptsOneObject(t *testing.T) {
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"known":"value"}`))
	w := httptest.NewRecorder()
	var dst struct {
		Known string `json:"known"`
	}
	if err := decodeJSON(w, req, &dst); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if dst.Known != "value" {
		t.Fatalf("got %q", dst.Known)
	}
}
