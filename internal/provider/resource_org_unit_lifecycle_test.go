package googleworkspace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// orgUnitFixture exercises Google's real wire encoding without live credentials.
type orgUnitFixture func(*http.Request) (*http.Response, error)

func (f orgUnitFixture) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func orgUnitResponse(code int, value string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(value))}
}

// TestOrgUnitUpdatePreservesChanges proves moves and explicit description clearing reach the API.
func TestOrgUnitUpdatePreservesChanges(t *testing.T) {
	for _, parent := range []string{"parent_org_unit_id", "parent_org_unit_path"} {
		t.Run(parent, func(t *testing.T) {
			r := resourceOrgUnit()
			state := &terraform.InstanceState{ID: "id:child", Attributes: map[string]string{"id": "id:child", "org_unit_id": "id:child", "name": "Child", "description": "old description", "parent_org_unit_id": "id:old", "parent_org_unit_path": "/Old", "org_unit_path": "/Old/Child", "block_inheritance": "true", "etag": "old"}}
			value, wire := "id:new", "parentOrgUnitId"
			if parent == "parent_org_unit_path" {
				value, wire = "/New", "parentOrgUnitPath"
			}
			config := terraform.NewResourceConfigRaw(map[string]interface{}{"name": "Renamed", "description": "", parent: value, "block_inheritance": false})
			writes := 0
			client := &apiClient{Customer: "C01234567", client: &http.Client{Transport: orgUnitFixture(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "admin.googleapis.com" || !strings.HasSuffix(request.URL.Path, "/orgunits/id:child") {
					t.Fatalf("unexpected endpoint %s", request.URL)
				}
				if request.Method == "PUT" {
					writes++
					var body map[string]interface{}
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body[wire] != value || body["description"] != "" || body["name"] != "Renamed" {
						t.Errorf("lost desired update: %#v", body)
					}
					if _, present := body["description"]; !present {
						t.Error("description clearing was omitted")
					}
					if value, present := body["blockInheritance"]; !present || value != false {
						t.Error("explicit false was omitted")
					}
				}
				if request.Header.Get("If-None-Match") != "" {
					return orgUnitResponse(304, ""), nil
				}
				return orgUnitResponse(200, `{"orgUnitId":"id:child","name":"Renamed","description":"","parentOrgUnitId":"id:new","parentOrgUnitPath":"/New","orgUnitPath":"/New/Renamed","etag":"new"}`), nil
			})}}
			diff, err := r.Diff(context.Background(), state, config, client)
			if err != nil {
				t.Fatal(err)
			}
			updated, diagnostics := r.Apply(context.Background(), state, diff, client)
			if diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			if writes != 1 || updated.ID != state.ID || updated.Attributes["description"] != "" {
				t.Fatalf("unexpected updated state: %#v", updated)
			}
		})
	}
}

// TestOrgUnitCreateReportsConsistencyFailure prevents a later read masking a failed wait.
func TestOrgUnitCreateReportsConsistencyFailure(t *testing.T) {
	reads := 0
	r := resourceOrgUnit()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]interface{}{"name": "Child", "parent_org_unit_path": "/"})
	client := &apiClient{Customer: "C01234567", client: &http.Client{Transport: orgUnitFixture(func(request *http.Request) (*http.Response, error) {
		if request.Method == "GET" {
			reads++
			if reads == 1 {
				return orgUnitResponse(403, `{"error":{"code":403,"message":"synthetic-secret"}}`), nil
			}
		}
		return orgUnitResponse(200, `{"orgUnitId":"id:child","name":"Child","parentOrgUnitPath":"/","etag":"one"}`), nil
	})}}
	if diagnostics := r.CreateContext(context.Background(), d, client); !diagnostics.HasError() {
		t.Fatal("consistency failure was hidden")
	}
}

// TestOrgUnitFailureRedaction ensures normal diagnostics never forward remote bodies.
func TestOrgUnitFailureRedaction(t *testing.T) {
	r := resourceOrgUnit()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]interface{}{"name": "Child", "parent_org_unit_path": "/"})
	d.SetId("id:child")
	client := &apiClient{Customer: "C01234567", client: &http.Client{Transport: orgUnitFixture(func(request *http.Request) (*http.Response, error) {
		return orgUnitResponse(403, `{"error":{"code":403,"message":"synthetic-secret"}}`), nil
	})}}
	for _, operation := range []string{"read", "create", "update", "delete"} {
		callback := r.ReadContext
		switch operation {
		case "create":
			callback = schema.ReadContextFunc(r.CreateContext)
		case "update":
			callback = schema.ReadContextFunc(r.UpdateContext)
		case "delete":
			callback = schema.ReadContextFunc(r.DeleteContext)
		}
		diagnostics := callback(context.Background(), d, client)
		if !diagnostics.HasError() {
			t.Fatal("permission denial accepted")
		}
		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Summary+diagnostic.Detail, "synthetic-secret") {
				t.Fatal("provider body exposed")
			}
		}
	}
}

// TestOrgUnitDataSourceFailureRedaction covers both supported lookup selectors.
func TestOrgUnitDataSourceFailureRedaction(t *testing.T) {
	for _, field := range []string{"org_unit_id", "org_unit_path"} {
		t.Run(field, func(t *testing.T) {
			definition := dataSourceOrgUnit()
			value := "id:child"
			if field == "org_unit_path" {
				value = "/Child"
			}
			data := schema.TestResourceDataRaw(t, definition.Schema, map[string]interface{}{field: value})
			client := &apiClient{Customer: "C01234567", client: &http.Client{Transport: orgUnitFixture(func(request *http.Request) (*http.Response, error) {
				return orgUnitResponse(403, `{"error":{"code":403,"message":"synthetic-secret"}}`), nil
			})}}
			diagnostics := definition.ReadContext(context.Background(), data, client)
			if !diagnostics.HasError() {
				t.Fatal("lookup denial accepted")
			}
			for _, diagnostic := range diagnostics {
				if strings.Contains(diagnostic.Summary+diagnostic.Detail, "synthetic-secret") {
					t.Fatal("lookup exposed remote body")
				}
			}
		})
	}
}
