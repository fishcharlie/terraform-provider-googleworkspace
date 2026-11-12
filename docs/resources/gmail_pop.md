---
page_title: "googleworkspace_gmail_pop Resource"
---

# googleworkspace_gmail_pop

Manage existing POP preferences with `primary_email`, required `access_window`
(`disabled`, `fromNowOn`, `allMail`) and required `disposition` (`leaveInInbox`,
`archive`, `trash`, `markRead`). Import using the mailbox email address. Reads
never access messages. No defaults are invented; removing this singleton forgets
ownership without resetting the mailbox. Protect adopted mailboxes with
`lifecycle { prevent_destroy = true }`.

See the [Gmail POP settings API](https://developers.google.com/workspace/gmail/api/reference/rest/v1/PopSettings).
