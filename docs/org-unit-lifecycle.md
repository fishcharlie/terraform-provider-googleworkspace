# Organizational unit lifecycle correction

The baseline OU updater omits parent changes and serializes an empty description
as an absent field. A successful update can therefore leave the old values in
Google. Create also ignores a failed consistency wait, and normal error
messages can include remote response content.

This version preserves the existing resource schema/import IDs and fixes only
OU request encoding, consistency failure propagation and safe diagnostics.
Existing domain/user/Gmail/role resources retain their implementations. Parent
changes are sent using the configured ID or path selector, never both computed
selectors. Empty descriptions use Google's ForceSendFields contract. Imports and
refresh issue GETs only; normal deletion deletes the OU and existing production
users must protect adopted OUs with prevent_destroy. The deprecated
blockInheritance API field does not represent current policy inheritance.

| Acceptance | Implementation | Executable evidence | Release/operational boundary |
| --- | --- | --- | --- |
| Preserve import/refresh and stable IDs | Existing OU callbacks/schema | Actual encrypted OpenTofu import and clean refresh against synthetic HTTPS API | Production import-only plan and clean follow-up required by consumer |
| Apply parent-ID/path moves and clear descriptions | OU updater, typed Google SDK | Before-fix SDK fixture loses both fields; corrected request/SDK lifecycle and native provider update converge | No production move or description update authorized by this fix |
| Fail correctly on eventual consistency or denied access | Create/read/update callbacks | SDK create-consistency denial; native first-read404, permission denial and expired token | No guessed absence or duplicate-create recovery |
| Preserve credential privacy | OU diagnostic boundary | Synthetic echoed-secret denial never appears in provider diagnostics | Debug logging is not a supported credential-safe mode |
| Correct deletion and critical identity protection | Existing delete callback; consumer lifecycle | Native provider delete, refresh and protected-resource destruction rejection | Fixture deletion only; no tenant OU deletion authorized |
| Generic reproducible release | Existing deterministic builder | Three-platform builds independently reproduced, exact Linux artifact lifecycle | Requires separate source/release publication authorization; no tenant data belongs in this repository |

Primary contracts: [Directory OU schema](https://developers.google.com/workspace/admin/directory/reference/rest/v1/orgunits)
and [OU updates](https://developers.google.com/workspace/admin/directory/v1/guides/manage-org-units).
