package nrdp

import (
	"encoding/json"
	"encoding/xml"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		ct       string
		form     url.Values
		expected string
	}{
		{"xml form", "application/x-www-form-urlencoded", url.Values{"XMLDATA": {"<xml/>"}}, FormatXMLForm},
		{"json form", "application/x-www-form-urlencoded", url.Values{"JSONDATA": {`{}`}}, FormatJSONForm},
		{"form no data", "application/x-www-form-urlencoded", url.Values{}, FormatUnknown},
		{"text/xml", "text/xml", nil, FormatRawXML},
		{"application/xml", "application/xml", nil, FormatRawXML},
		{"application/json", "application/json", nil, FormatRawJSON},
		{"text/plain", "text/plain", nil, FormatUnknown},
		{"json with charset", "application/json; charset=utf-8", nil, FormatRawJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFormat(tt.ct, tt.form)
			if got != tt.expected {
				t.Errorf("DetectFormat(%q) = %q, want %q", tt.ct, got, tt.expected)
			}
		})
	}
}

const testXML = `<?xml version="1.0" encoding="utf-8"?>
<checkresults>
  <checkresult type="service" checktype="1">
    <hostname>web01</hostname>
    <servicename>HTTP</servicename>
    <state>0</state>
    <output>OK - 200 response</output>
  </checkresult>
  <checkresult type="service" checktype="1">
    <hostname>db01</hostname>
    <servicename>MySQL</servicename>
    <state>2</state>
    <output>CRITICAL - connection refused</output>
  </checkresult>
</checkresults>`

const testJSON = `{
  "checkresults": [
    {
      "type": "service",
      "hostname": "web01",
      "servicename": "HTTP",
      "status": 0,
      "output": "OK - 200 response"
    },
    {
      "type": "service",
      "hostname": "db01",
      "servicename": "MySQL",
      "status": 2,
      "output": "CRITICAL - connection refused"
    }
  ]
}`

func TestParseXML(t *testing.T) {
	results, err := parseXML([]byte(testXML))
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Hostname != "web01" || results[0].Servicename != "HTTP" || results[0].Status != 0 {
		t.Errorf("result[0] = %+v", results[0])
	}
	if results[1].Hostname != "db01" || results[1].Servicename != "MySQL" || results[1].Status != 2 {
		t.Errorf("result[1] = %+v", results[1])
	}
	if results[0].Output != "OK - 200 response" {
		t.Errorf("result[0].Output = %q", results[0].Output)
	}
}

func TestParseJSON(t *testing.T) {
	results, err := parseJSON([]byte(testJSON))
	if err != nil {
		t.Fatalf("parseJSON: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Hostname != "web01" || results[0].Servicename != "HTTP" || results[0].Status != 0 {
		t.Errorf("result[0] = %+v", results[0])
	}
	if results[1].Hostname != "db01" || results[1].Status != 2 {
		t.Errorf("result[1] = %+v", results[1])
	}
}

func TestParsePayloadXMLForm(t *testing.T) {
	form := url.Values{"XMLDATA": {testXML}}
	results, err := ParsePayload(FormatXMLForm, nil, form)
	if err != nil {
		t.Fatalf("ParsePayload XMLForm: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestParsePayloadJSONForm(t *testing.T) {
	form := url.Values{"JSONDATA": {testJSON}}
	results, err := ParsePayload(FormatJSONForm, nil, form)
	if err != nil {
		t.Fatalf("ParsePayload JSONForm: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestParsePayloadRawXML(t *testing.T) {
	results, err := ParsePayload(FormatRawXML, []byte(testXML), nil)
	if err != nil {
		t.Fatalf("ParsePayload RawXML: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestParsePayloadRawJSON(t *testing.T) {
	results, err := ParsePayload(FormatRawJSON, []byte(testJSON), nil)
	if err != nil {
		t.Fatalf("ParsePayload RawJSON: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestClampStatus(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, 0}, {1, 1}, {2, 2}, {3, 3},
		{-1, 3}, {4, 3}, {100, 3},
	}
	for _, tt := range tests {
		got := clampStatus(tt.in)
		if got != tt.want {
			t.Errorf("clampStatus(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	now := time.Now()

	// RFC3339
	ts := "2024-02-10T12:34:56Z"
	got := parseTimestamp(ts)
	if got.Year() != 2024 || got.Month() != 2 || got.Day() != 10 {
		t.Errorf("RFC3339: got %v", got)
	}

	// ISO8601 without Z
	got = parseTimestamp("2024-02-10T12:34:56")
	if got.Year() != 2024 {
		t.Errorf("ISO8601: got %v", got)
	}

	// Date-time with space
	got = parseTimestamp("2024-02-10 12:34:56")
	if got.Year() != 2024 {
		t.Errorf("datetime: got %v", got)
	}

	// Epoch string
	got = parseTimestamp("1707567296")
	if got.Year() < 2024 {
		t.Errorf("epoch: got %v", got)
	}

	// Empty -> now
	got = parseTimestamp("")
	if got.Sub(now) > time.Second {
		t.Errorf("empty: got %v, want ~now", got)
	}

	// Garbage -> now
	got = parseTimestamp("not-a-date")
	if got.Sub(now) > time.Second {
		t.Errorf("garbage: got %v, want ~now", got)
	}
}

func TestSanitizeOutput(t *testing.T) {
	// Strips \r, \t, null bytes
	input := "OK\r\n\toutput\x00here"
	got := sanitizeOutput(input)
	if strings.ContainsRune(got, '\r') || strings.ContainsRune(got, '\t') || strings.ContainsRune(got, 0) {
		t.Errorf("sanitizeOutput did not strip control chars: %q", got)
	}
	// Preserves newline
	if !strings.Contains(got, "\n") {
		t.Errorf("sanitizeOutput stripped newline: %q", got)
	}
	// Preserves pipe and perfdata
	input2 := "OK - response | time=0.5s"
	got2 := sanitizeOutput(input2)
	if got2 != input2 {
		t.Errorf("sanitizeOutput(%q) = %q", input2, got2)
	}
}

func TestFormatResponseXML(t *testing.T) {
	body, ct := FormatResponse(FormatRawXML, "ABC", 200, "Processing 1 Results")
	if ct != "text/xml" {
		t.Errorf("content-type = %q, want text/xml", ct)
	}
	s := string(body)
	if !strings.Contains(s, xml.Header) {
		t.Errorf("missing XML header in %q", s)
	}
	if !strings.Contains(s, "<response>") {
		t.Errorf("missing <response> element in %q", s)
	}
	if !strings.Contains(s, "<id>ABC</id>") {
		t.Errorf("missing id in %q", s)
	}

	// Also works for XMLForm format
	body2, ct2 := FormatResponse(FormatXMLForm, "XYZ", 200, "ok")
	if ct2 != "text/xml" {
		t.Errorf("XMLForm ct = %q", ct2)
	}
	if !strings.Contains(string(body2), "<id>XYZ</id>") {
		t.Errorf("XMLForm body missing id")
	}
}

func TestFormatResponseJSON(t *testing.T) {
	body, ct := FormatResponse(FormatRawJSON, "DEF", 200, "Processing 2 Results")
	if ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var resp ResponseJSON
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "DEF" || resp.Status != 200 || resp.Message != "Processing 2 Results" {
		t.Errorf("resp = %+v", resp)
	}

	// Also works for JSONForm
	body2, ct2 := FormatResponse(FormatJSONForm, "GHI", 200, "ok")
	if ct2 != "application/json" {
		t.Errorf("JSONForm ct = %q", ct2)
	}
	_ = body2
}

func TestFormatResponsePlainText(t *testing.T) {
	body, ct := FormatResponse(FormatUnknown, "ZZZ", 500, "error message")
	if ct != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", ct)
	}
	if string(body) != "error message" {
		t.Errorf("body = %q", string(body))
	}
}

func TestGenerateRequestID(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := GenerateRequestID()
		if len(id) != 3 {
			t.Fatalf("len = %d, want 3", len(id))
		}
		for _, c := range id {
			if c < 'A' || c > 'Z' {
				t.Fatalf("char %c not in A-Z", c)
			}
		}
	}
}

func TestBuildSource(t *testing.T) {
	got := BuildSource("json", "192.168.1.1:12345")
	if got != "json://192.168.1.1:12345" {
		t.Errorf("BuildSource = %q", got)
	}

	// No port
	got2 := BuildSource("xml", "192.168.1.1")
	if got2 != "xml://192.168.1.1" {
		t.Errorf("BuildSource no port = %q", got2)
	}
}
