---
page_title: "googleworkspace_gmail_vacation Resource"
---

# googleworkspace_gmail_vacation

Manage existing vacation reply preferences. Import using `primary_email` as the
ID. `enable_auto_reply`, `restrict_to_contacts`, and `restrict_to_domain` are
required. `response_subject`, `response_body`, `response_format`, `start_time`,
and `end_time` are optional/computed, retaining observed values when omitted.

`response_format` is `plain_text` or `html`. Exactly one body representation is
sent. Import selects nonempty HTML when present, as Google's API gives HTML
precedence; otherwise it selects plain text. Gmail sanitizes HTML, so use valid
markup and review any normalization during refresh. Enabling replies requires
a nonempty subject or body.

Time boundaries are positive epoch milliseconds represented as strings. Absent
boundaries remain absent on the wire; epoch zero is not an unbounded sentinel.
Start must precede end when both are present. Removing a previously configured
boundary is rejected during plan and update: Google's documentation does not
establish clearing semantics, and this provider does not guess. Positive-boundary
updates and preserving existing boundaries are supported. This limitation does
not prevent importing mailboxes without boundaries. Review a supported API
contract and independent evidence before extending clearing behavior.

Removing this singleton forgets state ownership without disabling responses or
resetting any preference. Gmail exposes no delete-preferences API. All settings
reads avoid messages. See [VacationSettings](https://developers.google.com/workspace/gmail/api/reference/rest/v1/VacationSettings).
