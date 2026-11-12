# Gmail settings extension

This generic extension adds existing-mailbox IMAP, POP, language, automatic
forwarding and vacation settings resources. Each imports by mailbox email,
refreshes through the documented Gmail API, updates through the typed SDK, and
forgets ownership without resetting live preferences when deleted. Required
booleans and numbers are serialized explicitly; optional API fields retain
observed values. No new OAuth scopes or message APIs are introduced.

The provider signs separately for each mailbox. The caller must constrain the
customer, service-account identity and allowed scopes before invocation. Gmail
settings scopes allow writes during both planning and applying; provider resource
reads do not turn those scopes into enforced read-only credentials. Credential
and token schema fields are sensitive. Invalid credential diagnostics never
include submitted contents; resource diagnostics omit remote error bodies.

This extension does not manage forwarding-address verification or SMTP secrets.
The existing send-as and delegate resources remain unchanged. Do not infer that
an absent collection grants ownership of future records. Vacation-boundary
clearing is explicitly rejected until the API behavior is established.

| Acceptance | Owning code | Executable evidence |
| --- | --- | --- |
| Import/refresh convergence without remote mutation | `resource_gmail_settings.go` | `scripts/test-gmail-settings-lifecycle.py`: ten imports across two synthetic mailboxes, saved import apply, clean follow-up |
| Preserve false/zero and omitted values | Typed Gmail requests | Go wire tests and native encrypted OpenTofu lifecycle |
| Body-format ownership and optional boundaries | Vacation request and validation | Go serialization regression; native HTML/plain switching, timestamp preservation and clear rejection |
| Supported writes and failure reconciliation | Resource update/read callbacks | Native denied PUT, successful PUT followed by failed refresh, fresh reconciliation and transient read |
| Tenant/subject isolation and bounded API access | Existing per-mailbox signer | Native RSA-signed JWT verification, exact scopes, fixed audiences and approved synthetic subjects; unknown endpoints rejected |
| Safe removal | Singleton DeleteContext | Go import/forget test and native state-only deletion without preference writes |
| Credential diagnostics | Provider validation and settings diagnostics | Reproduced credential-echo regression, safe API-body diagnostics, native denied/expired/read failures |

Run Go tests with the pinned release toolchain. Build the local executable and run
the native script on Linux with OpenTofu 1.13.1, Python and `cryptography` installed:

```sh
go test ./...
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /tmp/workspace-provider .
python scripts/test-gmail-settings-lifecycle.py --provider /tmp/workspace-provider
```

The native test uses ephemeral synthetic RSA credentials and local HTTPS APIs.
Run it in a container without external networking. Go on macOS does not honor the
fixture's `SSL_CERT_FILE` trust boundary; do not install its CA into the host trust
store. This is fixture evidence, not live tenant adoption or a publication claim.
