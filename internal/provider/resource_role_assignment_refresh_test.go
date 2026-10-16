// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package googleworkspace

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// roleAssignmentFixture keeps the real SDK serialization while prohibiting network access.
type roleAssignmentFixture func(*http.Request) (*http.Response, error)

func (f roleAssignmentFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRoleAssignmentRefreshAssigneeType(t *testing.T) {
	for _, value := range []string{"USER", "user", "GROUP", "group", "", "unknown"} {
		t.Run(value, func(t *testing.T) {
			resource := resourceRoleAssignment()
			data := schema.TestResourceDataRaw(t, resource.Schema, map[string]interface{}{})
			data.SetId("2001")
			// Imported state has no configured default until refresh restores the API value.
			if err := data.Set("assignee_type", ""); err != nil {
				t.Fatal(err)
			}
			client := &apiClient{Customer: "test-customer", client: &http.Client{Transport: roleAssignmentFixture(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Host != "admin.googleapis.com" || request.URL.Path != "/admin/directory/v1/customer/test-customer/roleassignments/2001" {
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
				}
				body := fmt.Sprintf(`{"roleAssignmentId":"2001","roleId":"1001","assignedTo":"3001","assigneeType":%q,"scopeType":"CUSTOMER"}`, value)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			diagnostics := resourceRoleAssignmentRead(context.Background(), data, client)
			if value == "" || value == "unknown" {
				if !diagnostics.HasError() {
					t.Fatal("missing or unsupported assignee type must stop refresh")
				}
				return
			}
			if diagnostics.HasError() {
				t.Fatalf("refresh failed: %v", diagnostics)
			}
			if got := data.Get("assignee_type"); got != strings.ToUpper(value) {
				t.Fatalf("assignee type: got %q, want %q", got, strings.ToUpper(value))
			}
			if data.Get("assigned_to") != "3001" || data.Get("role_id") != "1001" || data.Get("scope_type") != "CUSTOMER" {
				t.Fatal("refresh changed assignment ownership")
			}
		})
	}
}
