package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStateXMLReturnsDefaultRegistersAndCustomQueryValues(t *testing.T) {
	state := newDeviceState(defaultMACAddress)
	req := httptest.NewRequest(http.MethodGet, "/state.xml?register2=x.x&register4=12.7", nil)
	recorder := httptest.NewRecorder()

	stateXMLHandler(state).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := recorder.Body.String()
	for _, expected := range []string{
		"<register1>0</register1>",
		"<register2>x.x</register2>",
		"<register3>0</register3>",
		"<register4>12.7</register4>",
		"<register5>0</register5>",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected response to contain %q, got %s", expected, body)
		}
	}
}
