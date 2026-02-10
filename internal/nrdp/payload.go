package nrdp

import (
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Format detection constants.
const (
	FormatXMLForm  = "xmlform"
	FormatJSONForm = "jsonform"
	FormatRawXML   = "xml"
	FormatRawJSON  = "json"
	FormatUnknown  = "unknown"
)

// XMLCheckResults is the top-level XML envelope for check results.
type XMLCheckResults struct {
	XMLName      xml.Name         `xml:"checkresults"`
	CheckResults []XMLCheckResult `xml:"checkresult"`
}

// XMLCheckResult is a single check result in XML format.
type XMLCheckResult struct {
	Type        string `xml:"type,attr"`
	Checktype   string `xml:"checktype,attr"`
	Hostname    string `xml:"hostname"`
	Servicename string `xml:"servicename"`
	State       int    `xml:"state"`
	Output      string `xml:"output"`
	Timestamp   string `xml:"timestamp"`
}

// JSONPayload is the top-level JSON envelope for check results.
type JSONPayload struct {
	CheckResults []JSONCheckResult `json:"checkresults"`
}

// JSONCheckResult is a single check result in JSON format.
type JSONCheckResult struct {
	Type        string `json:"type"`
	Hostname    string `json:"hostname"`
	Servicename string `json:"servicename"`
	Status      int    `json:"status"`
	Output      string `json:"output"`
	Timestamp   string `json:"timestamp"`
}

// NRDPResult is the normalized internal representation of a check result.
type NRDPResult struct {
	Hostname    string
	Servicename string
	Status      int
	Output      string
	Timestamp   time.Time
	Source      string // "{protocol}://{remote_ip}:{port}"
}

// ResponseXML is the XML response envelope.
type ResponseXML struct {
	XMLName xml.Name `xml:"response"`
	ID      string   `xml:"id"`
	Status  int      `xml:"status"`
	Message string   `xml:"message"`
}

// ResponseJSON is the JSON response envelope.
type ResponseJSON struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// DetectFormat determines the payload format from the Content-Type header and form data.
func DetectFormat(contentType string, formData url.Values) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	// Strip parameters (e.g. charset)
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}

	switch ct {
	case "application/x-www-form-urlencoded":
		if formData.Get("XMLDATA") != "" {
			return FormatXMLForm
		}
		if formData.Get("JSONDATA") != "" {
			return FormatJSONForm
		}
		return FormatUnknown
	case "text/xml", "application/xml":
		return FormatRawXML
	case "application/json":
		return FormatRawJSON
	default:
		return FormatUnknown
	}
}

// ParsePayload parses check results from the request body or form data according to the detected format.
func ParsePayload(format string, body []byte, formData url.Values) ([]NRDPResult, error) {
	switch format {
	case FormatXMLForm:
		data := formData.Get("XMLDATA")
		if data == "" {
			return nil, fmt.Errorf("empty XMLDATA field")
		}
		return parseXML([]byte(data))
	case FormatJSONForm:
		data := formData.Get("JSONDATA")
		if data == "" {
			return nil, fmt.Errorf("empty JSONDATA field")
		}
		return parseJSON([]byte(data))
	case FormatRawXML:
		return parseXML(body)
	case FormatRawJSON:
		return parseJSON(body)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func parseXML(data []byte) ([]NRDPResult, error) {
	var envelope XMLCheckResults
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("xml decode: %w", err)
	}
	results := make([]NRDPResult, len(envelope.CheckResults))
	for i, cr := range envelope.CheckResults {
		results[i] = NRDPResult{
			Hostname:    cr.Hostname,
			Servicename: cr.Servicename,
			Status:      clampStatus(cr.State),
			Output:      sanitizeOutput(cr.Output),
			Timestamp:   parseTimestamp(cr.Timestamp),
		}
	}
	return results, nil
}

func parseJSON(data []byte) ([]NRDPResult, error) {
	var payload JSONPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	results := make([]NRDPResult, len(payload.CheckResults))
	for i, cr := range payload.CheckResults {
		results[i] = NRDPResult{
			Hostname:    cr.Hostname,
			Servicename: cr.Servicename,
			Status:      clampStatus(cr.Status),
			Output:      sanitizeOutput(cr.Output),
			Timestamp:   parseTimestamp(cr.Timestamp),
		}
	}
	return results, nil
}

// clampStatus ensures the status value is in the valid 0-3 range.
func clampStatus(s int) int {
	if s < 0 || s > 3 {
		return 3
	}
	return s
}

// parseTimestamp tries multiple time formats and falls back to time.Now().
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(epoch, 0)
	}
	return time.Now()
}

// sanitizeOutput strips control characters except newline (0x0A).
func sanitizeOutput(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != 0x0A {
			return -1
		}
		return r
	}, s)
}

// FormatResponse builds the response body and content type for the given format.
func FormatResponse(format string, id string, status int, message string) ([]byte, string) {
	switch format {
	case FormatXMLForm, FormatRawXML:
		resp := ResponseXML{
			ID:      id,
			Status:  status,
			Message: message,
		}
		body, err := xml.Marshal(resp)
		if err != nil {
			return []byte(message), "text/plain"
		}
		return append([]byte(xml.Header), body...), "text/xml"
	case FormatJSONForm, FormatRawJSON:
		resp := ResponseJSON{
			ID:      id,
			Status:  status,
			Message: message,
		}
		body, err := json.Marshal(resp)
		if err != nil {
			return []byte(message), "text/plain"
		}
		return body, "application/json"
	default:
		return []byte(message), "text/plain"
	}
}

// GenerateRequestID returns 3 random uppercase ASCII letters.
func GenerateRequestID() string {
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		return "ERR"
	}
	for i := range b {
		b[i] = 'A' + b[i]%26
	}
	return string(b)
}

// BuildSource constructs a source string in the form "{format}://{ip}:{port}".
func BuildSource(format string, remoteAddr string) string {
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return fmt.Sprintf("%s://%s", format, remoteAddr)
	}
	return fmt.Sprintf("%s://%s:%s", format, host, port)
}
