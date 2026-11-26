package ingest

import (
	"testing"
)

// The envelope parser reads raw bytes from anyone holding a DSN: it must never panic, and every item it
// returns must come from the input.
func FuzzParseEnvelope(f *testing.F) {
	f.Add([]byte("{\"event_id\":\"9ec79c33ec9942ab8353589fcb2e04dc\"}\n{\"type\":\"event\",\"length\":13}\n{\"message\":1}\n"))
	f.Add([]byte("{}\n{\"type\":\"event\"}\n{\"message\":\"no length\"}\n"))
	f.Add([]byte("{}\n{\"type\":\"attachment\",\"length\":999999999}\nabc"))
	f.Add([]byte("{}\n{\"type\":\"log\",\"length\":-5}\n{}\n"))
	f.Add([]byte("{}\n{\"type\":\"event\",\"length\":1.5e308}\n{}\n"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		env, err := ParseEnvelope(data)
		if err != nil {
			return
		}
		total := 0
		for _, it := range env.Items {
			total += len(it.Payload)
		}
		if total > len(data) {
			t.Fatalf("items hold %d bytes from a %d-byte envelope", total, len(data))
		}
	})
}
