package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

var datapointNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*$`)

type datapoint struct {
	Key   string
	Value string
}

type deviceState struct {
	mu           sync.RWMutex
	values       map[string]string
	valueOrder   []string
	exposed      map[string]struct{}
	exposedOrder []string
}

type responseHTTPVersion string

const (
	responseHTTP09    responseHTTPVersion = "0.9"
	responseHTTP10    responseHTTPVersion = "1.0"
	responseHTTP11    responseHTTPVersion = "1.1"
	defaultMACAddress                     = "DE:AD:BE:EF:00:01"
)

func newDeviceState(serialNumber string) *deviceState {
	defaults := []datapoint{
		{Key: "digitalInput1", Value: "0"},
		{Key: "digitalInput2", Value: "0"},
		{Key: "digitalInput3", Value: "1"},
		{Key: "digitalInput4", Value: "0"},
		{Key: "relay1", Value: "0"},
		{Key: "relay2", Value: "0"},
		{Key: "relay3", Value: "0"},
		{Key: "relay4", Value: "0"},
		{Key: "analogInput1", Value: "0"},
		{Key: "analogInput2", Value: "0"},
		{Key: "analogInput3", Value: "0"},
		{Key: "analogInput4", Value: "0"},
		{Key: "vin", Value: "30.5"},
		{Key: "register1", Value: "0"},
		{Key: "utcTime", Value: "957741231"},
		{Key: "timezoneOffset", Value: "-21600"},
		{Key: "serialNumber", Value: serialNumber},
		{Key: "minRecRefresh", Value: "3"},
	}

	state := &deviceState{
		values:       make(map[string]string, len(defaults)),
		valueOrder:   make([]string, 0, len(defaults)),
		exposed:      make(map[string]struct{}, len(defaults)),
		exposedOrder: make([]string, 0, len(defaults)),
	}

	for _, point := range defaults {
		state.values[point.Key] = point.Value
		state.valueOrder = append(state.valueOrder, point.Key)
		state.exposed[point.Key] = struct{}{}
		state.exposedOrder = append(state.exposedOrder, point.Key)
	}

	return state
}

func (d *deviceState) updateFromQuery(query url.Values) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for key, values := range query {
		if !isValidDatapointName(key) || len(values) == 0 {
			continue
		}

		d.ensureKnownLocked(key)
		d.values[key] = values[len(values)-1]
		d.exposeLocked(key)
	}
}

func (d *deviceState) applyConfig(query url.Values) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if isTrue(query.Get("reset")) {
		d.resetExposureLocked()
	}

	if setRaw := query.Get("set"); setRaw != "" {
		keys := parseDatapointList(setRaw)
		d.exposed = make(map[string]struct{}, len(keys))
		d.exposedOrder = d.exposedOrder[:0]
		for _, key := range keys {
			d.ensureKnownLocked(key)
			d.exposeLocked(key)
		}
	}

	if addRaw := query.Get("add"); addRaw != "" {
		for _, key := range parseDatapointList(addRaw) {
			d.ensureKnownLocked(key)
			d.exposeLocked(key)
		}
	}

	if removeRaw := query.Get("remove"); removeRaw != "" {
		for _, key := range parseDatapointList(removeRaw) {
			d.hideLocked(key)
		}
	}
}

func (d *deviceState) snapshotExposed() []datapoint {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]datapoint, 0, len(d.exposedOrder))
	for _, key := range d.exposedOrder {
		if _, ok := d.exposed[key]; !ok {
			continue
		}
		value, ok := d.values[key]
		if !ok {
			continue
		}
		out = append(out, datapoint{Key: key, Value: value})
	}

	return out
}

func (d *deviceState) snapshotConfig() (available []string, exposed []string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	available = append([]string(nil), d.valueOrder...)

	exposed = make([]string, 0, len(d.exposedOrder))
	for _, key := range d.exposedOrder {
		if _, ok := d.exposed[key]; ok {
			exposed = append(exposed, key)
		}
	}

	return available, exposed
}

func (d *deviceState) ensureKnownLocked(key string) {
	if _, ok := d.values[key]; ok {
		return
	}
	d.values[key] = "0"
	d.valueOrder = append(d.valueOrder, key)
}

func (d *deviceState) exposeLocked(key string) {
	if _, ok := d.exposed[key]; ok {
		return
	}
	d.exposed[key] = struct{}{}
	d.exposedOrder = append(d.exposedOrder, key)
}

func (d *deviceState) hideLocked(key string) {
	if _, ok := d.exposed[key]; !ok {
		return
	}

	delete(d.exposed, key)

	filtered := d.exposedOrder[:0]
	for _, currentKey := range d.exposedOrder {
		if currentKey == key {
			continue
		}
		if _, ok := d.exposed[currentKey]; ok {
			filtered = append(filtered, currentKey)
		}
	}
	d.exposedOrder = filtered
}

func (d *deviceState) resetExposureLocked() {
	d.exposed = make(map[string]struct{}, len(d.valueOrder))
	d.exposedOrder = d.exposedOrder[:0]
	for _, key := range d.valueOrder {
		d.exposed[key] = struct{}{}
		d.exposedOrder = append(d.exposedOrder, key)
	}
}

func stateJSONHandler(state *deviceState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}

		state.updateFromQuery(r.URL.Query())
		datapoints := state.snapshotExposed()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := writeStateJSON(w, datapoints); err != nil {
			http.Error(w, "failed to write JSON response", http.StatusInternalServerError)
		}
	}
}

func stateXMLHandler(state *deviceState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}

		state.updateFromQuery(r.URL.Query())
		datapoints := state.snapshotExposed()

		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		if err := writeStateXML(w, datapoints); err != nil {
			http.Error(w, "failed to write XML response", http.StatusInternalServerError)
		}
	}
}

func configHandler(state *deviceState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}

		if len(r.URL.Query()) > 0 {
			state.applyConfig(r.URL.Query())
		}

		available, exposed := state.snapshotConfig()

		payload := map[string]any{
			"availableDatapoints": available,
			"exposedDatapoints":   exposed,
			"queryHelp": map[string]string{
				"set":    "Replace the exposed set: /config?set=relay1,vin",
				"add":    "Add datapoints: /config?add=relay4,register1",
				"remove": "Hide datapoints: /config?remove=relay3,relay4",
				"reset":  "Expose everything again: /config?reset=1",
			},
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			http.Error(w, "failed to write config response", http.StatusInternalServerError)
		}
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(
		"cbw-server\n" +
			"\n" +
			"GET /state.xml\n" +
			"GET /state.json\n" +
			"GET /config\n",
	))
}

func writeStateJSON(w io.Writer, datapoints []datapoint) error {
	var buf bytes.Buffer
	buf.WriteString("{\n")

	for i, point := range datapoints {
		keyJSON, err := json.Marshal(point.Key)
		if err != nil {
			return err
		}
		valueJSON, err := json.Marshal(point.Value)
		if err != nil {
			return err
		}

		buf.WriteString("  ")
		buf.Write(keyJSON)
		buf.WriteString(":")
		buf.Write(valueJSON)
		if i < len(datapoints)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	_, err := w.Write(buf.Bytes())
	return err
}

func writeStateXML(w io.Writer, datapoints []datapoint) error {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString("<datavalues>\n")

	for _, point := range datapoints {
		if !isValidDatapointName(point.Key) {
			continue
		}

		buf.WriteString("  <")
		buf.WriteString(point.Key)
		buf.WriteString(">")

		if err := xml.EscapeText(&buf, []byte(point.Value)); err != nil {
			return err
		}

		buf.WriteString("</")
		buf.WriteString(point.Key)
		buf.WriteString(">\n")
	}

	buf.WriteString("</datavalues>\n")
	_, err := w.Write(buf.Bytes())
	return err
}

func parseDatapointList(raw string) []string {
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key == "" || !isValidDatapointName(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	return keys
}

func isTrue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isValidDatapointName(name string) bool {
	return datapointNamePattern.MatchString(name)
}

func methodNotAllowed(w http.ResponseWriter, method string) {
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func parseRuntimeFlags() (responseHTTPVersion, string, error) {
	forceHTTP09 := flag.Bool("http0.9", false, "serve body-only HTTP/0.9 style responses")
	forceHTTP10 := flag.Bool("http1.0", false, "serve HTTP/1.0 status line and headers")
	forceHTTP11 := flag.Bool("http1.1", false, "serve HTTP/1.1 status line and headers (default)")
	macAddress := flag.String("mac", defaultMACAddress, "serialNumber MAC address exposed by /state.*")
	flag.Parse()

	version := responseHTTP11
	selected := 0

	if *forceHTTP09 {
		version = responseHTTP09
		selected++
	}

	if *forceHTTP10 {
		version = responseHTTP10
		selected++
	}

	if *forceHTTP11 {
		version = responseHTTP11
		selected++
	}

	if selected > 1 {
		return "", "", fmt.Errorf("flags --http0.9, --http1.0, and --http1.1 are mutually exclusive")
	}

	normalizedMAC, err := normalizeMACAddress(*macAddress)
	if err != nil {
		return "", "", err
	}

	return version, normalizedMAC, nil
}

func normalizeMACAddress(raw string) (string, error) {
	parsedMAC, err := net.ParseMAC(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid --mac value %q: %w", raw, err)
	}

	if len(parsedMAC) != 6 {
		return "", fmt.Errorf("invalid --mac value %q: expected 6-byte MAC address", raw)
	}

	return strings.ToUpper(parsedMAC.String()), nil
}

func withForcedResponseVersion(version responseHTTPVersion, next http.Handler) http.Handler {
	switch version {
	case responseHTTP09:
		return forceHTTP09Responses(next)
	case responseHTTP10:
		return forceHTTP10Responses(next)
	case responseHTTP11:
		return next
	default:
		return next
	}
}

func forceHTTP10Responses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)

		result := recorder.Result()
		defer result.Body.Close()

		body, err := io.ReadAll(result.Body)
		if err != nil {
			log.Printf("failed to build HTTP/1.0 body: %v", err)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
			return
		}

		conn, rw, err := hijacker.Hijack()
		if err != nil {
			log.Printf("failed to hijack connection: %v", err)
			return
		}
		defer conn.Close()

		headers := result.Header.Clone()
		headers.Del("Transfer-Encoding")
		headers.Set("Connection", "close")
		if headers.Get("Content-Length") == "" {
			headers.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		}

		statusText := http.StatusText(result.StatusCode)
		if statusText == "" {
			statusText = "status"
		}

		if _, err := rw.WriteString(fmt.Sprintf("HTTP/1.0 %d %s\r\n", result.StatusCode, statusText)); err != nil {
			log.Printf("failed to write HTTP/1.0 status line: %v", err)
			return
		}

		for key, values := range headers {
			for _, value := range values {
				if _, err := rw.WriteString(fmt.Sprintf("%s: %s\r\n", key, value)); err != nil {
					log.Printf("failed to write HTTP/1.0 header: %v", err)
					return
				}
			}
		}

		if _, err := rw.WriteString("\r\n"); err != nil {
			log.Printf("failed to finish HTTP/1.0 headers: %v", err)
			return
		}

		if _, err := rw.Write(body); err != nil {
			log.Printf("failed to write HTTP/1.0 response: %v", err)
			return
		}

		if err := rw.Flush(); err != nil {
			log.Printf("failed to flush HTTP/1.0 response: %v", err)
		}
	})
}

func forceHTTP09Responses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)

		result := recorder.Result()
		defer result.Body.Close()

		body, err := io.ReadAll(result.Body)
		if err != nil {
			log.Printf("failed to build HTTP/0.9 body: %v", err)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
			return
		}

		conn, rw, err := hijacker.Hijack()
		if err != nil {
			log.Printf("failed to hijack connection: %v", err)
			return
		}
		defer conn.Close()

		if _, err := rw.Write(body); err != nil {
			log.Printf("failed to write HTTP/0.9 response: %v", err)
			return
		}

		if err := rw.Flush(); err != nil {
			log.Printf("failed to flush HTTP/0.9 response: %v", err)
		}
	})
}

func main() {
	responseVersion, macAddress, err := parseRuntimeFlags()
	if err != nil {
		log.Fatal(err)
	}

	state := newDeviceState(macAddress)

	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/state.json", stateJSONHandler(state))
	mux.HandleFunc("/state.xml", stateXMLHandler(state))
	mux.HandleFunc("/config", configHandler(state))

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	handler := withForcedResponseVersion(responseVersion, mux)

	addr := ":" + port
	log.Printf("cbw-server listening on %s (responses as HTTP/%s, serialNumber=%s)", addr, responseVersion, macAddress)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
