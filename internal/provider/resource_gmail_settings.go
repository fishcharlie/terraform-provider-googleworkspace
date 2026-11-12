// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package googleworkspace

import (
	"context"
	"fmt"
	"net/mail"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

// gmailSettingDefinition shares lifecycle ownership while retaining typed SDK requests.
type gmailSettingDefinition struct {
	fields []gmailSettingField
	read   func(context.Context, *gmail.Service) (map[string]interface{}, error)
	update func(context.Context, *gmail.Service, *schema.ResourceData) error
}
type gmailSettingField struct {
	name, wire string
	schema     *schema.Schema
}

// gmailSetting describes only documented settings endpoints, never message APIs.
func gmailSetting(kind string) gmailSettingDefinition {
	switch kind {
	case "imap":
		return gmailSettingDefinition{
			fields: []gmailSettingField{
				{name: "enabled", wire: "enabled", schema: &schema.Schema{Description: "Gmail enabled setting. Match observed configuration before adoption.", Type: schema.TypeBool, Required: true}},
				{name: "auto_expunge", wire: "autoExpunge", schema: &schema.Schema{Description: "Gmail autoExpunge setting. Match observed configuration before adoption.", Type: schema.TypeBool, Required: true}},
				{name: "expunge_behavior", wire: "expungeBehavior", schema: &schema.Schema{Description: "Gmail expungeBehavior setting. Match observed configuration before adoption.", Type: schema.TypeString, Required: true, ValidateFunc: validation.StringInSlice([]string{"archive", "trash", "deleteForever"}, false)}},
				{name: "max_folder_size", wire: "maxFolderSize", schema: &schema.Schema{Description: "Gmail maxFolderSize setting. Match observed configuration before adoption.", Type: schema.TypeInt, Required: true, ValidateFunc: validation.IntInSlice([]int{0, 1000, 2000, 5000, 10000})}},
			},
			read: func(ctx context.Context, s *gmail.Service) (map[string]interface{}, error) {
				v, err := s.Users.Settings.GetImap("me").Context(ctx).Do()
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"enabled":          v.Enabled,
					"auto_expunge":     v.AutoExpunge,
					"expunge_behavior": v.ExpungeBehavior,
					"max_folder_size":  int(v.MaxFolderSize),
				}, nil
			},
			update: func(ctx context.Context, s *gmail.Service, d *schema.ResourceData) error {
				_, err := s.Users.Settings.UpdateImap("me", &gmail.ImapSettings{
					Enabled:         d.Get("enabled").(bool),
					AutoExpunge:     d.Get("auto_expunge").(bool),
					ExpungeBehavior: d.Get("expunge_behavior").(string),
					MaxFolderSize:   int64(d.Get("max_folder_size").(int)),
					ForceSendFields: []string{"Enabled", "AutoExpunge", "ExpungeBehavior", "MaxFolderSize"},
				}).Context(ctx).Do()
				return err
			},
		}
	case "pop":
		return gmailSettingDefinition{
			fields: []gmailSettingField{
				{name: "access_window", wire: "accessWindow", schema: &schema.Schema{Description: "Gmail accessWindow setting. Match observed configuration before adoption.", Type: schema.TypeString, Required: true, ValidateFunc: validation.StringInSlice([]string{"disabled", "fromNowOn", "allMail"}, false)}},
				{name: "disposition", wire: "disposition", schema: &schema.Schema{Description: "Gmail disposition setting. Match observed configuration before adoption.", Type: schema.TypeString, Required: true, ValidateFunc: validation.StringInSlice([]string{"leaveInInbox", "archive", "trash", "markRead"}, false)}},
			},
			read: func(ctx context.Context, s *gmail.Service) (map[string]interface{}, error) {
				v, err := s.Users.Settings.GetPop("me").Context(ctx).Do()
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"access_window": v.AccessWindow,
					"disposition":   v.Disposition,
				}, nil
			},
			update: func(ctx context.Context, s *gmail.Service, d *schema.ResourceData) error {
				_, err := s.Users.Settings.UpdatePop("me", &gmail.PopSettings{
					AccessWindow:    d.Get("access_window").(string),
					Disposition:     d.Get("disposition").(string),
					ForceSendFields: []string{"AccessWindow", "Disposition"},
				}).Context(ctx).Do()
				return err
			},
		}
	case "language":
		return gmailSettingDefinition{
			fields: []gmailSettingField{
				{name: "display_language", wire: "displayLanguage", schema: &schema.Schema{Description: "Gmail displayLanguage setting. Match observed configuration before adoption.", Type: schema.TypeString, Required: true}},
			},
			read: func(ctx context.Context, s *gmail.Service) (map[string]interface{}, error) {
				v, err := s.Users.Settings.GetLanguage("me").Context(ctx).Do()
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"display_language": v.DisplayLanguage,
				}, nil
			},
			update: func(ctx context.Context, s *gmail.Service, d *schema.ResourceData) error {
				_, err := s.Users.Settings.UpdateLanguage("me", &gmail.LanguageSettings{
					DisplayLanguage: d.Get("display_language").(string),
					ForceSendFields: []string{"DisplayLanguage"},
				}).Context(ctx).Do()
				return err
			},
		}
	case "auto_forwarding":
		return gmailSettingDefinition{
			fields: []gmailSettingField{
				{name: "enabled", wire: "enabled", schema: &schema.Schema{Description: "Gmail enabled setting. Match observed configuration before adoption.", Type: schema.TypeBool, Required: true}},
				{name: "email_address", wire: "emailAddress", schema: &schema.Schema{Description: "Gmail emailAddress setting. Match observed configuration before adoption.", Type: schema.TypeString, Optional: true, Computed: true}},
				{name: "disposition", wire: "disposition", schema: &schema.Schema{Description: "Gmail disposition setting. Match observed configuration before adoption.", Type: schema.TypeString, Optional: true, Computed: true, ValidateFunc: validation.StringInSlice([]string{"", "leaveInInbox", "archive", "trash", "markRead"}, false)}},
			},
			read: func(ctx context.Context, s *gmail.Service) (map[string]interface{}, error) {
				v, err := s.Users.Settings.GetAutoForwarding("me").Context(ctx).Do()
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"enabled":       v.Enabled,
					"email_address": v.EmailAddress,
					"disposition":   v.Disposition,
				}, nil
			},
			update: func(ctx context.Context, s *gmail.Service, d *schema.ResourceData) error {
				_, err := s.Users.Settings.UpdateAutoForwarding("me", &gmail.AutoForwarding{
					Enabled:         d.Get("enabled").(bool),
					EmailAddress:    d.Get("email_address").(string),
					Disposition:     d.Get("disposition").(string),
					ForceSendFields: []string{"Enabled"},
				}).Context(ctx).Do()
				return err
			},
		}
	case "vacation":
		return gmailSettingDefinition{
			fields: []gmailSettingField{
				{name: "enable_auto_reply", wire: "enableAutoReply", schema: &schema.Schema{Type: schema.TypeBool, Required: true, Description: "Whether vacation replies are enabled."}},
				{name: "response_subject", wire: "responseSubject", schema: &schema.Schema{Type: schema.TypeString, Optional: true, Computed: true, Description: "Subject prepended to vacation responses."}},
				{name: "response_body", schema: &schema.Schema{Type: schema.TypeString, Optional: true, Computed: true, Description: "Response body in the selected format. Only one body representation is sent."}},
				{name: "response_format", schema: &schema.Schema{Type: schema.TypeString, Optional: true, Computed: true, Description: "Body format: plain_text or html. Imported HTML takes precedence as documented by Gmail.", ValidateFunc: validation.StringInSlice([]string{"plain_text", "html"}, false)}},
				{name: "restrict_to_contacts", wire: "restrictToContacts", schema: &schema.Schema{Type: schema.TypeBool, Required: true, Description: "Restrict replies to contacts."}},
				{name: "restrict_to_domain", wire: "restrictToDomain", schema: &schema.Schema{Type: schema.TypeBool, Required: true, Description: "Restrict replies to the mailbox domain."}},
				{name: "start_time", wire: "startTime", schema: &schema.Schema{Type: schema.TypeString, Optional: true, Computed: true, Description: "Optional positive epoch milliseconds. Empty means omit this boundary from the request.", ValidateFunc: validateVacationTime}},
				{name: "end_time", wire: "endTime", schema: &schema.Schema{Type: schema.TypeString, Optional: true, Computed: true, Description: "Optional positive epoch milliseconds. Empty means omit this boundary from the request.", ValidateFunc: validateVacationTime}},
			},
			read: func(ctx context.Context, s *gmail.Service) (map[string]interface{}, error) {
				v, err := s.Users.Settings.GetVacation("me").Context(ctx).Do()
				if err != nil {
					return nil, err
				}
				body, format := v.ResponseBodyPlainText, "plain_text"
				if v.ResponseBodyHtml != "" {
					body, format = v.ResponseBodyHtml, "html"
				}
				return map[string]interface{}{"enable_auto_reply": v.EnableAutoReply, "response_subject": v.ResponseSubject, "response_body": body, "response_format": format, "restrict_to_contacts": v.RestrictToContacts, "restrict_to_domain": v.RestrictToDomain, "start_time": vacationTime(v.StartTime), "end_time": vacationTime(v.EndTime)}, nil
			},
			update: func(ctx context.Context, s *gmail.Service, d *schema.ResourceData) error {
				v := vacationRequest(d)
				_, err := s.Users.Settings.UpdateVacation("me", v).Context(ctx).Do()
				return err
			},
		}
	default:
		panic("unregistered Gmail settings resource")
	}
}

// validateVacationTime avoids inventing an epoch-zero sentinel for Google's optional boundaries.
func validateVacationTime(value interface{}, _ string) ([]string, []error) {
	text := value.(string)
	if text == "" {
		return nil, nil
	}
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number <= 0 || strconv.FormatInt(number, 10) != text {
		return nil, []error{fmt.Errorf("expected positive epoch milliseconds or an empty omitted boundary")}
	}
	return nil, nil
}
func vacationTime(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

// validateVacationChange refuses undocumented boundary clearing rather than silently changing its meaning.
func validateVacationChange(values interface {
	Get(string) interface{}
	GetChange(string) (interface{}, interface{})
}) error {
	for _, name := range []string{"start_time", "end_time"} {
		previous, current := values.GetChange(name)
		if before, ok := previous.(string); ok && before != "" && current == "" {
			return fmt.Errorf("clearing an existing vacation %s is not supported without verified Gmail clearing semantics", name)
		}
	}
	start, _ := strconv.ParseInt(values.Get("start_time").(string), 10, 64)
	end, _ := strconv.ParseInt(values.Get("end_time").(string), 10, 64)
	if start > 0 && end > 0 && start >= end {
		return fmt.Errorf("vacation start_time must precede end_time")
	}
	if values.Get("enable_auto_reply").(bool) && values.Get("response_subject").(string) == "" && values.Get("response_body").(string) == "" {
		return fmt.Errorf("enabled vacation replies require a subject or response body")
	}
	return nil
}

// vacationRequest sends exactly one body format and leaves absent timestamps absent.
func vacationRequest(d *schema.ResourceData) *gmail.VacationSettings {
	start, _ := strconv.ParseInt(d.Get("start_time").(string), 10, 64)
	end, _ := strconv.ParseInt(d.Get("end_time").(string), 10, 64)
	v := &gmail.VacationSettings{EnableAutoReply: d.Get("enable_auto_reply").(bool), ResponseSubject: d.Get("response_subject").(string), RestrictToContacts: d.Get("restrict_to_contacts").(bool), RestrictToDomain: d.Get("restrict_to_domain").(bool), StartTime: start, EndTime: end, ForceSendFields: []string{"EnableAutoReply", "ResponseSubject", "RestrictToContacts", "RestrictToDomain"}}
	if d.Get("response_format").(string) == "html" {
		v.ResponseBodyHtml = d.Get("response_body").(string)
		v.ForceSendFields = append(v.ForceSendFields, "ResponseBodyHtml")
	} else {
		v.ResponseBodyPlainText = d.Get("response_body").(string)
		v.ForceSendFields = append(v.ForceSendFields, "ResponseBodyPlainText")
	}
	return v
}

// gmailSettingDiagnostic omits remote bodies and credential-bearing transport errors.
func gmailSettingDiagnostic(err error) diag.Diagnostics {
	if remote, ok := err.(*googleapi.Error); ok {
		return diag.Errorf("Gmail settings request failed (HTTP %d); verify permissions and configuration.", remote.Code)
	}
	return diag.Errorf("Gmail settings request failed; verify credentials, connectivity and configuration.")
}

// validateMailboxID rejects display names and composite IDs before selecting a delegated subject.
func validateMailboxID(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || strings.ContainsAny(value, ":/\\ \t\r\n") {
		return fmt.Errorf("expected a mailbox email address as the import ID")
	}
	return nil
}

// resourceGmailSetting never invents defaults and forgets ownership without resetting the mailbox.
func resourceGmailSetting(kind string) *schema.Resource {
	definition := gmailSetting(kind)
	fields := map[string]*schema.Schema{
		"primary_email": {Type: schema.TypeString, Required: true, ForceNew: true, Description: "Mailbox to impersonate. Import using this email address.", ValidateFunc: func(value interface{}, _ string) ([]string, []error) {
			if err := validateMailboxID(value.(string)); err != nil {
				return nil, []error{err}
			}
			return nil, nil
		}},
	}
	for _, field := range definition.fields {
		fields[field.name] = field.schema
	}
	read := func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		service, diagnostics := meta.(*apiClient).NewGmailService(ctx, d.Get("primary_email").(string))
		if diagnostics.HasError() {
			return diag.Errorf("Could not authenticate the mailbox settings client.")
		}
		values, err := definition.read(ctx, service)
		if err != nil {
			return gmailSettingDiagnostic(err)
		}
		for name, value := range values {
			if err := d.Set(name, value); err != nil {
				return diag.Errorf("Could not record Gmail settings state.")
			}
		}
		d.SetId(d.Get("primary_email").(string))
		return nil
	}
	update := func(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
		service, diagnostics := meta.(*apiClient).NewGmailService(ctx, d.Get("primary_email").(string))
		if diagnostics.HasError() {
			return diag.Errorf("Could not authenticate the mailbox settings client.")
		}
		if kind == "vacation" {
			if err := validateVacationChange(d); err != nil {
				return diag.FromErr(err)
			}
		}
		if kind == "auto_forwarding" && d.Get("enabled").(bool) && (d.Get("email_address").(string) == "" || d.Get("disposition").(string) == "") {
			return diag.Errorf("Enabled forwarding requires a verified email_address and disposition.")
		}
		if err := definition.update(ctx, service, d); err != nil {
			return gmailSettingDiagnostic(err)
		}
		d.SetId(d.Get("primary_email").(string))
		return read(ctx, d, meta)
	}
	return &schema.Resource{
		CustomizeDiff: func(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
			if kind == "vacation" {
				// Values derived from another resource are checked once concrete during update.
				for _, name := range []string{"enable_auto_reply", "response_subject", "response_body", "start_time", "end_time"} {
					if !d.NewValueKnown(name) {
						return nil
					}
				}
				return validateVacationChange(d)
			}
			return nil
		},
		Description: "Manages existing Gmail " + kind + " preferences. Import by mailbox email before adoption. Removing this singleton only forgets ownership; it preserves live preferences. Gmail settings scopes allow writes during planning as well as applying.",
		Schema:      fields, CreateContext: update, ReadContext: read, UpdateContext: update,
		DeleteContext: func(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
			d.SetId("")
			return nil
		},
		Importer: &schema.ResourceImporter{StateContext: func(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
			if err := validateMailboxID(d.Id()); err != nil {
				return nil, err
			}
			if err := d.Set("primary_email", d.Id()); err != nil {
				return nil, err
			}
			return []*schema.ResourceData{d}, nil
		}},
	}
}
