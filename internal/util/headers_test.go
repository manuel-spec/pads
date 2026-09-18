package util_test

import (
	"net/http"
	"strings"
	"testing"

	"pads/internal/util"
)

func TestValidateHeadersAcceptsSessionHeaders(t *testing.T) {
	got, err := util.ValidateHeaders(map[string]string{
		"cookie":     "session=abc",
		"USER-AGENT": "Mozilla/5.0",
		"Referer":    "https://example.test/page",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	// Names are canonicalised so the downloader's Set calls cannot end up with
	// two spellings of the same header.
	for _, name := range []string{"Cookie", "User-Agent", "Referer"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing canonical header %q in %v", name, got)
		}
	}
}

func TestValidateHeadersRejectsUnlistedHeaders(t *testing.T) {
	// Range and Host are managed by the downloader and the transport; allowing
	// either would let a caller corrupt segmenting or redirect the request.
	for _, name := range []string{"Range", "Host", "Content-Length", "Connection", "X-Whatever"} {
		if _, err := util.ValidateHeaders(map[string]string{name: "value"}); err == nil {
			t.Errorf("header %q was accepted", name)
		}
	}
}

func TestValidateHeadersRejectsInjection(t *testing.T) {
	cases := map[string]string{
		"carriage return": "a\r\nX-Injected: 1",
		"newline":         "a\nX-Injected: 1",
		"null":            "a\x00b",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := util.ValidateHeaders(map[string]string{"Cookie": value}); err == nil {
				t.Fatal("expected the value to be rejected")
			}
		})
	}
}

func TestValidateHeadersRejectsOversizedValue(t *testing.T) {
	if _, err := util.ValidateHeaders(map[string]string{"Cookie": strings.Repeat("a", 9000)}); err == nil {
		t.Fatal("expected an oversized value to be rejected")
	}
}

func TestValidateHeadersDoesNotMutateInput(t *testing.T) {
	in := map[string]string{"cookie": "session=abc"}
	if _, err := util.ValidateHeaders(in); err != nil {
		t.Fatal(err)
	}
	if len(in) != 1 || in["cookie"] != "session=abc" {
		t.Fatalf("input was modified: %v", in)
	}
}

func TestApplyHeadersDoesNotOverrideRange(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test/f", nil)
	if err != nil {
		t.Fatal(err)
	}

	// The downloader applies headers first and sets Range afterwards.
	util.ApplyHeaders(req, map[string]string{"Cookie": "session=abc"})
	req.Header.Set("Range", "bytes=0-99")

	if req.Header.Get("Range") != "bytes=0-99" {
		t.Fatalf("Range = %q", req.Header.Get("Range"))
	}
	if req.Header.Get("Cookie") != "session=abc" {
		t.Fatalf("Cookie = %q", req.Header.Get("Cookie"))
	}
}
