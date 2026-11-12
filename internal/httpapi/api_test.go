package httpapi

import (
	"bytes"
	"net/http/httptest"
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
