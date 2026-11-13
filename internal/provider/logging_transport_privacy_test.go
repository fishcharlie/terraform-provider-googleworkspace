// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package googleworkspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
)

// diagnosticFixture exercises the real wrappers without sending synthetic secrets off-machine.
type diagnosticFixture func(*http.Request) (*http.Response, error)

func (f diagnosticFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// captureDiagnosticLogs restores the process logger because SDK diagnostics are shared state.
func captureDiagnosticLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("TF_LOG", "DEBUG")
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

func TestDiagnosticTransportPreservesWireBodiesWithoutLoggingPrivateData(t *testing.T) {
	output := captureDiagnosticLogs(t)
	requestBody := "<entry><property value=\"request-body-secret\"/></entry>"
	responseBody := "<entry><property value=\"response-body-secret\"/></entry>"
	transport := NewTransportWithScrubbedLogs("fixture", diagnosticFixture(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil || string(body) != requestBody || request.Header.Get("Authorization") != "Bearer bearer-secret" {
			t.Fatal("diagnostics changed the wire request")
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Set-Cookie": {"cookie-secret"}}, Body: io.NopCloser(strings.NewReader(responseBody))}, nil
	}))
	request, _ := http.NewRequestWithContext(context.Background(), "PUT", "https://example.test/path-secret?token=query-secret", strings.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer bearer-secret")
	request.Header.Set("X-Custom", "header-secret")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != responseBody {
		t.Fatal("diagnostics consumed the wire response")
	}
	for _, secret := range []string{"request-body-secret", "response-body-secret", "bearer-secret", "cookie-secret", "header-secret", "path-secret", "query-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("private material reached diagnostic output: %s", secret)
		}
	}
	if !strings.Contains(output.String(), "PUT") || !strings.Contains(output.String(), "200") {
		t.Fatal("safe method/status diagnostics were lost")
	}
}

func TestRetryDiagnosticsDoNotLogRemoteErrorBodies(t *testing.T) {
	output := captureDiagnosticLogs(t)
	transport := NewTransportWithDefaultRetries(diagnosticFixture(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"code":403,"message":"remote-body-secret"}}`))}, nil
	}))
	request, _ := http.NewRequest("GET", "https://example.test/", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal("response behavior changed")
	}
	response.Body.Close()
	if strings.Contains(output.String(), "remote-body-secret") {
		t.Fatal("retry diagnostics exposed the remote response body")
	}
}

// unreadableDiagnosticBody proves even local body errors cannot embed secret-bearing text in logs.
type unreadableDiagnosticBody struct{}

func (unreadableDiagnosticBody) Read([]byte) (int, error) { return 0, errors.New("body-read-secret") }
func (unreadableDiagnosticBody) Close() error             { return nil }

func TestDiagnosticReadFailuresNeverExposeUnderlyingErrors(t *testing.T) {
	output := captureDiagnosticLogs(t)
	transport := NewTransportWithDefaultRetries(diagnosticFixture(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport-secret")
	}))
	request, _ := http.NewRequest("POST", "https://example.test/", unreadableDiagnosticBody{})
	_, _ = transport.RoundTrip(request)
	for _, secret := range []string{"body-read-secret", "transport-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("diagnostics exposed underlying error: %s", secret)
		}
	}
}

func TestRetryableFailureDiagnosticsKeepClassificationWithoutPrivateMessages(t *testing.T) {
	output := captureDiagnosticLogs(t)
	failures := []error{
		&googleapi.Error{Code: 500, Message: "retryable-secret"},
		&googleapi.Error{Code: 502, Body: "retryable-secret"},
		&googleapi.Error{Code: 503, Message: "retryable-secret"},
		&googleapi.Error{Code: 401, Body: "Login Required retryable-secret"},
		&googleapi.Error{Code: 403, Body: "Not Authorized to access this resource/api retryable-secret"},
		&googleapi.Error{Code: 403, Message: "Quota exceeded retryable-secret"},
		&googleapi.Error{Code: 429, Message: "retryable-secret"},
		&googleapi.Error{Code: 412, Body: "retryable-secret"},
		&googleapi.Error{Code: 400, Body: "Invalid Input: resource_id retryable-secret"},
		errors.New("retryable-secret: connection reset by peer"),
	}
	for _, failure := range failures {
		if !isRetryableError(failure, defaultErrorRetryPredicates...) {
			t.Fatal("retry classification changed")
		}
	}
	if !isRetryableError(errors.New("retryable-secret"), func(error) (bool, string) { return true, "predicate-secret" }) {
		t.Fatal("additional predicate changed")
	}
	for _, secret := range []string{"retryable-secret", "predicate-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("retryable diagnostic exposed private data: %s", secret)
		}
	}
}

func TestComposedDiagnosticFailurePreservesErrorAndHidesUnknownMethod(t *testing.T) {
	output := captureDiagnosticLogs(t)
	failure := errors.New("transport-secret")
	logging := NewTransportWithScrubbedLogs("fixture", diagnosticFixture(func(request *http.Request) (*http.Response, error) {
		if request.Method != "method-secret" {
			t.Fatal("logging changed the actual method")
		}
		return nil, failure
	}))
	transport := NewTransportWithDefaultRetries(logging)
	request, _ := http.NewRequest("POST", "https://example.test/", nil)
	request.Method = "method-secret"
	_, err := transport.RoundTrip(request)
	if !errors.Is(err, failure) {
		t.Fatal("transport error identity changed")
	}
	for _, secret := range []string{"method-secret", "transport-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("composed wrapper exposed private data: %s", secret)
		}
	}
}
