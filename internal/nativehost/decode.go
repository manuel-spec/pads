package nativehost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// decodeRequest parses one request strictly: an unknown field or trailing data
// means the extension and the host disagree about the protocol, which is worth
// surfacing rather than silently ignoring.
func decodeRequest(payload []byte, req *Request) error {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		return errors.New("request must contain exactly one JSON value")
	}
	if req.Type == "" {
		return errors.New("request type is required")
	}
	return nil
}
