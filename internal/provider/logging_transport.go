// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package googleworkspace

import (
	"log"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
)

type loggingTransport struct {
	name      string
	transport http.RoundTripper
}

// RoundTrip records operation metadata without consuming or logging credential-bearing wire data.
func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if logging.IsDebugOrHigher() {
		// SDK paths, queries, headers and arbitrary JSON/XML bodies can all contain secrets.
		method := req.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		default:
			method = "OTHER"
		}
		log.Printf("[DEBUG] %s API request method=%s", t.name, method)
	}
	resp, err := t.transport.RoundTrip(req)
	if logging.IsDebugOrHigher() {
		if err != nil {
			// Transport errors may embed URLs, response content or credential refresh failures.
			log.Printf("[DEBUG] %s API transport failed; details withheld", t.name)
		} else if resp != nil {
			log.Printf("[DEBUG] %s API response status=%d", t.name, resp.StatusCode)
		}
	}
	return resp, err
}

// NewTransportWithScrubbedLogs wraps the authenticated transport without changing its wire behavior.
func NewTransportWithScrubbedLogs(name string, t http.RoundTripper) *loggingTransport {
	return &loggingTransport{name, t}
}
