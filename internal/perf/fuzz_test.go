package perf

import (
	"encoding/json"
	"testing"

	"github.com/errorlenshq/errorlens/internal/scrub"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func FuzzFromSDKEvent(f *testing.F) {
	f.Add([]byte(`{"type":"transaction","transaction":"GET /x","start_timestamp":1764150000.1,"timestamp":1764150000.4,"contexts":{"trace":{"trace_id":"0af7651916cd43dd8448eb211c80319c","span_id":"b7ad6b7169203331","op":"http.server"}},"spans":[{"span_id":"1","parent_span_id":"b7ad6b7169203331","op":"db","start_timestamp":1764150000.2,"timestamp":1764150000.3}]}`))
	f.Add([]byte(`{"spans":[null,1,{"timestamp":"x"}],"contexts":{"trace":7},"start_timestamp":1e308,"timestamp":-1e308}`))
	f.Add([]byte(`{"contexts":{"trace":{"trace_id":"zz"}},"spans":{"a":1}}`))
	sc := scrub.New(scrub.DefaultConfig)
	f.Fuzz(func(t *testing.T, data []byte) {
		var ev map[string]any
		if json.Unmarshal(data, &ev) != nil || ev == nil {
			return
		}
		tx, err := FromSDKEvent(ev)
		if err != nil || tx == nil {
			return
		}
		Scrub(tx, sc)
	})
}

func FuzzFromOTLP(f *testing.F) {
	f.Add([]byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"0af7651916cd43dd8448eb211c80319c","spanId":"b7ad6b7169203331","name":"GET /x","kind":2,"startTimeUnixNano":"1","endTimeUnixNano":"2","events":[{"name":"exception","attributes":[{"key":"exception.type","value":{"stringValue":"E"}}]}]}]}]}]}`), true)
	f.Add([]byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"spanId":"","parentSpanId":"x","startTimeUnixNano":"18446744073709551615","endTimeUnixNano":"0"}]}]}]}`), true)
	f.Add([]byte{0x0a, 0x00}, false)
	sc := scrub.New(scrub.DefaultConfig)
	f.Fuzz(func(t *testing.T, data []byte, isJSON bool) {
		req := &coltrace.ExportTraceServiceRequest{}
		var err error
		if isJSON {
			err = protojson.Unmarshal(data, req)
		} else {
			err = proto.Unmarshal(data, req)
		}
		if err != nil {
			return
		}
		for _, tx := range FromOTLP(req) {
			Scrub(tx, sc)
		}
	})
}
