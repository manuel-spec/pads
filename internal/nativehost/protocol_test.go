package nativehost

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

func framed(payload string) []byte {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(payload)))
	buf.Write(header)
	buf.WriteString(payload)
	return buf.Bytes()
}

func TestReadMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMessage(&buf, []byte(`{"type":"health"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readMessage(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != `{"type":"health"}` {
		t.Fatalf("payload = %s", got)
	}
}

func TestReadMessageRejectsOversizedFrame(t *testing.T) {
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, maxMessageBytes+1)

	_, err := readMessage(bytes.NewReader(header))
	if err == nil {
		t.Fatal("expected an error for an oversized frame")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want a size complaint", err)
	}
}

func TestReadMessageReportsCleanEOF(t *testing.T) {
	// A closed stream between messages is how the browser signals shutdown and
	// must be distinguishable from a truncated frame.
	if _, err := readMessage(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("err = %v, want io.EOF", err)
	}

	if _, err := readMessage(bytes.NewReader([]byte{1, 2})); err == io.EOF {
		t.Fatal("a truncated header must not read as a clean EOF")
	}
}

func TestDecodeRequestIsStrict(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{name: "valid", payload: `{"type":"start","url":"http://x/y"}`},
		{name: "unknown field", payload: `{"type":"start","nope":1}`, wantErr: true},
		{name: "trailing value", payload: `{"type":"health"}{"type":"health"}`, wantErr: true},
		{name: "missing type", payload: `{"url":"http://x/y"}`, wantErr: true},
		{name: "not json", payload: `garbage`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req Request
			err := decodeRequest([]byte(tc.payload), &req)
			if tc.wantErr && err == nil {
				t.Fatal("expected an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
