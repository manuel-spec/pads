package util

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// forwardableHeaders is the set a caller may attach to a download.
//
// This is a whitelist rather than a blacklist on purpose. The headers worth
// forwarding are the ones that carry a browser session, and everything else is
// either managed by the downloader (Range, Accept-Encoding), managed by the
// transport (Host, Connection, Content-Length), or a way to make PADS issue
// requests the caller could not otherwise make.
var forwardableHeaders = map[string]bool{
	"Cookie":          true,
	"Referer":         true,
	"User-Agent":      true,
	"Authorization":   true,
	"Accept":          true,
	"Accept-Language": true,
}

// ForwardableHeaderNames lists the accepted headers in a stable order, for
// error messages and documentation.
func ForwardableHeaderNames() []string {
	names := make([]string, 0, len(forwardableHeaders))
	for name := range forwardableHeaders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateHeaders canonicalises and checks caller-supplied request headers.
// It returns a new map and never mutates its argument.
func ValidateHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(headers))
	for name, value := range headers {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if canonical == "" {
			return nil, fmt.Errorf("header name cannot be empty")
		}
		if !forwardableHeaders[canonical] {
			return nil, fmt.Errorf(
				"header %q cannot be forwarded; allowed headers are %s",
				canonical, strings.Join(ForwardableHeaderNames(), ", "),
			)
		}
		if err := validateHeaderValue(canonical, value); err != nil {
			return nil, err
		}
		out[canonical] = value
	}
	return out, nil
}

// validateHeaderValue rejects values that would let a caller inject extra
// headers or terminate the request line.
func validateHeaderValue(name, value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("header %q contains a line break", name)
	}
	if len(value) > maxHeaderValueBytes {
		return fmt.Errorf("header %q is longer than %d bytes", name, maxHeaderValueBytes)
	}
	return nil
}

const maxHeaderValueBytes = 8 << 10

// ApplyHeaders sets validated headers on a request. It is called before the
// downloader sets Range, so a forwarded header can never override the byte
// range a segment depends on.
func ApplyHeaders(req *http.Request, headers map[string]string) {
	for name, value := range headers {
		req.Header.Set(name, value)
	}
}
