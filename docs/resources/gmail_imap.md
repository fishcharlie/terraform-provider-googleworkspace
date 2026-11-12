---
page_title: "googleworkspace_gmail_imap Resource"
---

# googleworkspace_gmail_imap

Adopts the existing IMAP preferences of a Gmail mailbox. Use the delegated Gmail
settings scopes and import before managing an existing mailbox. This resource
reads settings only; it never reads mailbox messages.

```hcl
resource "googleworkspace_gmail_imap" "existing" {
  primary_email    = "person@example.test"
  enabled          = false
  auto_expunge     = true
  expunge_behavior = "archive"
  max_folder_size  = 0
  lifecycle { prevent_destroy = true }
}
import {
  to = googleworkspace_gmail_imap.existing
  id = "person@example.test"
}
```

All four IMAP preference fields are required; there are no implicit provider
defaults. Allowed folder limits are 0, 1000, 2000, 5000, or 10000. Expunge behavior
is `archive`, `trash`, or `deleteForever`. Match the observed settings before
reviewing the import plan. Removing this singleton forgets state ownership and
preserves the mailbox preferences; Gmail exposes no delete-preferences API.

See the [Gmail IMAP settings API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/ImapSettings).
