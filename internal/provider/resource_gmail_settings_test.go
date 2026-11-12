package googleworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// TestGmailSettingsWire exercises Google's typed SDK so false and zero cannot silently disappear.
func TestGmailSettingsWire(t *testing.T) {
	cases := []struct {
		kind, response string
		values         map[string]interface{}
	}{
		{"imap", `{"enabled":false,"autoExpunge":false,"expungeBehavior":"archive","maxFolderSize":0}`, map[string]interface{}{"enabled": false, "auto_expunge": false, "expunge_behavior": "archive", "max_folder_size": 0}},
		{"pop", `{"accessWindow":"disabled","disposition":"leaveInInbox"}`, map[string]interface{}{"access_window": "disabled", "disposition": "leaveInInbox"}},
		{"language", `{"displayLanguage":"en"}`, map[string]interface{}{"display_language": "en"}},
		{"auto_forwarding", `{"enabled":false}`, map[string]interface{}{"enabled": false}},
		{"vacation", `{"enableAutoReply":false,"restrictToContacts":false,"restrictToDomain":false}`, map[string]interface{}{"enable_auto_reply": false, "restrict_to_contacts": false, "restrict_to_domain": false}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			var writes []map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					writes = append(writes, body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			service, err := gmail.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
			if err != nil {
				t.Fatal(err)
			}
			definition := gmailSetting(tc.kind)
			d := schema.TestResourceDataRaw(t, resourceGmailSetting(tc.kind).Schema, tc.values)
			if err := definition.update(context.Background(), service, d); err != nil {
				t.Fatal(err)
			}
			if len(writes) != 1 {
				t.Fatalf("writes=%d", len(writes))
			}
			for _, field := range definition.fields {
				if field.wire != "" && field.name != "start_time" && field.name != "end_time" && !(tc.kind == "auto_forwarding" && field.name != "enabled") {
					if _, ok := writes[0][field.wire]; !ok {
						t.Errorf("missing explicit field %s", field.wire)
					}
				}
			}
			values, err := definition.read(context.Background(), service)
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range tc.values {
				if got := values[name]; got != want {
					t.Errorf("%s=%v; want %v", name, got, want)
				}
			}
		})
	}
}

// TestGmailSettingImportAndForget verifies adoption and removal never mutate mailbox preferences.
func TestGmailSettingImportAndForget(t *testing.T) {
	for _, kind := range []string{"imap", "pop", "language", "auto_forwarding", "vacation"} {
		resource := resourceGmailSetting(kind)
		d := schema.TestResourceDataRaw(t, resource.Schema, nil)
		d.SetId("person@example.test")
		states, err := resource.Importer.StateContext(context.Background(), d, nil)
		if err != nil || len(states) != 1 || d.Get("primary_email") != "person@example.test" {
			t.Fatalf("%s: import failed: %v", kind, err)
		}
		if diags := resource.DeleteContext(context.Background(), d, nil); diags.HasError() || d.Id() != "" {
			t.Fatalf("%s: forget failed", kind)
		}
	}
}

// TestSettingsFailuresRedactProviderBodies prevents API errors from reflecting credentials.
func TestSettingsFailuresRedactProviderBodies(t *testing.T) {
	for _, err := range []error{&googleapi.Error{Code: 403, Body: "private fixture credential", Message: "private fixture credential"}, fmt.Errorf("private fixture credential")} {
		for _, diagnostic := range gmailSettingDiagnostic(err) {
			if strings.Contains(diagnostic.Summary+diagnostic.Detail, "private fixture credential") {
				t.Fatal("remote diagnostic leaked")
			}
		}
	}
}

// TestGmailSettingsSchema ensures the actual SDK provider accepts every resource schema.
func TestGmailSettingsSchema(t *testing.T) {
	if err := New("test")().InternalValidate(); err != nil {
		t.Fatal(err)
	}
}

// TestVacationWirePreservesOptionalSemantics covers the documented HTML precedence and optional dates.
func TestVacationWirePreservesOptionalSemantics(t *testing.T) {
	for _, format := range []string{"plain_text", "html"} {
		d := schema.TestResourceDataRaw(t, resourceGmailSetting("vacation").Schema, map[string]interface{}{"response_body": "dynamic <body>", "response_format": format})
		bytes, err := json.Marshal(vacationRequest(d))
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(bytes, &body); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"startTime", "endTime"} {
			if _, present := body[field]; present {
				t.Fatalf("absent boundary serialized: %s", field)
			}
		}
		selected, other := "responseBodyPlainText", "responseBodyHtml"
		if format == "html" {
			selected, other = other, selected
		}
		if body[selected] != "dynamic <body>" {
			t.Fatalf("selected body missing: %s", bytes)
		}
		if _, present := body[other]; present {
			t.Fatalf("alternate body overrides selected format: %s", bytes)
		}
	}
}

// TestInvalidCredentialsNeverEchoContents protects failures before any resource callback runs.
func TestInvalidCredentialsNeverEchoContents(t *testing.T) {
	secret := "synthetic-private-key-fragment"
	diagnostics := validateCredentials(`{"private_key":"`+secret+`",invalid}`, nil)
	if !diagnostics.HasError() {
		t.Fatal("invalid credentials accepted")
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Summary+diagnostic.Detail, secret) {
			t.Fatal("credential parser echoed private contents")
		}
	}
}

// TestDisabledForwardingOmitsAbsentDestination never invents an empty enum or unverified address.
func TestDisabledForwardingOmitsAbsentDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 1 || body["enabled"] != false {
			t.Errorf("unexpected disabled forwarding fields: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":false}`))
	}))
	defer server.Close()
	service, err := gmail.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	d := schema.TestResourceDataRaw(t, resourceGmailSetting("auto_forwarding").Schema, map[string]interface{}{"enabled": false})
	if err := gmailSetting("auto_forwarding").update(context.Background(), service, d); err != nil {
		t.Fatal(err)
	}
}
