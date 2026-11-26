package logs

import (
	"testing"
	"time"

	"github.com/errorlenshq/errorlens/internal/scrub"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func FuzzParseEnvelopeItem(f *testing.F) {
	f.Add([]byte(`{"items":[{"timestamp":1764150000.5,"level":"info","body":"Order {id}","attributes":{"id":{"value":42,"type":"integer"},"sentry.release":{"value":"x","type":"string"}}}]}`))
	f.Add([]byte(`{"items":[{"timestamp":"x","level":7,"body":null,"attributes":[1,2],"severity_number":1e400}]}`))
	f.Add([]byte(`{"items":null}`))
	f.Add([]byte(`[]`))
	sc := scrub.New(scrub.DefaultConfig)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, payload []byte) {
		recs, err := ParseEnvelopeItem(payload)
		if err != nil {
			return
		}
		_ = Clean(recs, sc, now)
	})
}

func FuzzFromOTLP(f *testing.F) {
	f.Add([]byte(`{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1764150000000000000","severityNumber":17,"body":{"stringValue":"boom"},"traceId":"0af7651916cd43dd8448eb211c80319c"}]}]}]}`), true)
	f.Add([]byte{0x0a, 0x02, 0x0a, 0x00}, false)
	sc := scrub.New(scrub.DefaultConfig)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, data []byte, isJSON bool) {
		req := &collogs.ExportLogsServiceRequest{}
		var err error
		if isJSON {
			err = protojson.Unmarshal(data, req)
		} else {
			err = proto.Unmarshal(data, req)
		}
		if err != nil {
			return
		}
		_ = Clean(FromOTLP(req), sc, now)
	})
}
