---
page_title: "googleworkspace_gmail_auto_forwarding Resource"
---

# googleworkspace_gmail_auto_forwarding

Manage existing automatic forwarding with `primary_email`, required `enabled`,
and optional/computed `email_address` and `disposition` (`leaveInInbox`, `archive`,
`trash`, `markRead`). Import using the mailbox email address. Enabling requires
both fields and an address Google has already verified. This resource cannot
create a forwarding address, send a verification message, or bypass verification.
An API verification/permission failure is reported without replacing the address.

Imported disabled forwarding can omit its absent destination and disposition.
Removing this singleton forgets ownership and preserves the live preference.
Use `prevent_destroy` to guard state ownership. Gmail settings scopes permit
writes even during planning; use separately controlled phase identities and
review exact saved plans.

See the [Gmail automatic forwarding API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/AutoForwarding).
