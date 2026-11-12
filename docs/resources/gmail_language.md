---
page_title: "googleworkspace_gmail_language Resource"
---

# googleworkspace_gmail_language

Manage `primary_email` and required `display_language`, matching the Gmail API's
language code. Import using the mailbox email address. Removing this singleton
forgets state ownership without resetting language preferences. No mailbox
messages are read.

See the [Gmail language settings API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/LanguageSettings).
