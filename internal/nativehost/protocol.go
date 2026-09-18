// Package nativehost implements the browser native-messaging bridge. A browser
// extension cannot read the daemon's control file or hold its bearer token, so
// it speaks to this host over stdio instead and the host makes the loopback
// call on its behalf.
package nativehost

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// maxMessageBytes caps a single framed message. Browsers refuse to deliver
// anything near this size, so a larger frame means a desynchronised stream
// rather than a legitimate request.
const maxMessageBytes = 1 << 20

// Request is one command from the extension.
type Request struct {
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
	ID       string `json:"id,omitempty"`
}

// Response is the host's reply. Every reply carries OK so the extension never
// has to infer success from the shape of the payload.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Running bool   `json:"running"`
	Addr    string `json:"addr,omitempty"`
	ID      string `json:"id,omitempty"`
	Output  string `json:"output,omitempty"`
	Jobs    []Job  `json:"jobs,omitempty"`
}

// Job is the subset of a daemon job the extension renders.
type Job struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Output string `json:"output"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Bytes  int64  `json:"bytes_done"`
	Total  int64  `json:"total_size"`
}

// errorResponse builds a failed reply. The extension shows Error verbatim, so
// it has to read as a sentence rather than a Go error chain.
func errorResponse(err error) Response {
	return Response{OK: false, Error: err.Error()}
}

// readMessage reads one length-prefixed frame.
//
// Native messaging frames a message as a 4-byte length in the host's native
// byte order followed by UTF-8 JSON. Every platform the browsers ship on is
// little-endian, which is why that order is fixed here rather than probed.
func readMessage(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("truncated message header: %w", err)
		}
		return nil, err
	}

	length := binary.LittleEndian.Uint32(header[:])
	if length == 0 {
		return nil, fmt.Errorf("empty message")
	}
	if length > maxMessageBytes {
		return nil, fmt.Errorf("message of %d bytes exceeds the %d byte limit", length, maxMessageBytes)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read message body: %w", err)
	}
	return payload, nil
}

// writeMessage writes one length-prefixed frame.
func writeMessage(w io.Writer, payload []byte) error {
	if len(payload) > maxMessageBytes {
		return fmt.Errorf("reply of %d bytes exceeds the %d byte limit", len(payload), maxMessageBytes)
	}

	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("write message header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write message body: %w", err)
	}
	return nil
}

func writeResponse(w io.Writer, resp Response) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode reply: %w", err)
	}
	return writeMessage(w, payload)
}
