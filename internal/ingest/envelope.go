// Package ingest implements the event ingestion endpoints used by SDKs
// (/api/{project}/envelope/ and /api/{project}/store/).
package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Envelope is a parsed SDK envelope: a JSON header line followed by
// items, each a JSON header line and a payload.
type Envelope struct {
	Header map[string]any
	Items  []Item
}

// Item is one envelope item.
type Item struct {
	Type    string
	Header  map[string]any
	Payload []byte
}

// ErrBadEnvelope is returned for malformed envelopes.
var ErrBadEnvelope = errors.New("malformed envelope")

// ParseEnvelope parses the envelope wire format. Item payloads use the
// explicit "length" header when present, otherwise run to the next newline.
func ParseEnvelope(data []byte) (*Envelope, error) {
	line, rest := nextLine(data)
	env := &Envelope{}
	if err := json.Unmarshal(line, &env.Header); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrBadEnvelope, err)
	}
	for len(bytes.TrimSpace(rest)) > 0 {
		var hdrLine []byte
		hdrLine, rest = nextLine(rest)
		if len(bytes.TrimSpace(hdrLine)) == 0 {
			continue
		}
		var hdr map[string]any
		if err := json.Unmarshal(hdrLine, &hdr); err != nil {
			return nil, fmt.Errorf("%w: item header: %v", ErrBadEnvelope, err)
		}
		item := Item{Header: hdr}
		item.Type, _ = hdr["type"].(string)

		if n, ok := lengthOf(hdr["length"]); ok {
			if n < 0 || n > len(rest) {
				return nil, fmt.Errorf("%w: item length %d exceeds body", ErrBadEnvelope, n)
			}
			item.Payload = rest[:n]
			rest = rest[n:]
			if len(rest) > 0 && rest[0] == '\n' {
				rest = rest[1:]
			}
		} else {
			item.Payload, rest = nextLine(rest)
		}
		env.Items = append(env.Items, item)
		if len(env.Items) > 1000 {
			return nil, fmt.Errorf("%w: too many items", ErrBadEnvelope)
		}
	}
	return env, nil
}

func nextLine(b []byte) (line, rest []byte) {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return bytes.TrimSuffix(b[:i], []byte("\r")), b[i+1:]
	}
	return b, nil
}

func lengthOf(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}
