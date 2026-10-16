# Terraform Provider Google Workspace
<a href="https://terraform.io">
    <img src="https://www.datocms-assets.com/2885/1620155116-brandhcterraformverticalcolor.svg" alt="Terraform logo" align="right" height="50" />
</a>

[![Releases](https://img.shields.io/github/release/fishcharlie/terraform-provider-googleworkspace.svg)](https://github.com/fishcharlie/terraform-provider-googleworkspace/releases)
[![LICENSE](https://img.shields.io/github/license/fishcharlie/terraform-provider-googleworkspace.svg)](https://github.com/fishcharlie/terraform-provider-googleworkspace/blob/release-0.12/LICENSE)

This Google Workspace provider for Terraform allows you to manage domains, users, and groups in your Google Workspace.

This downstream fork starts from [`SamuZad/googleworkspace` v0.12.0](https://github.com/SamuZad/terraform-provider-googleworkspace/releases/tag/v0.12.0), which descends from the archived HashiCorp provider. The focused release restores role-assignment assignee type during refresh so existing assignments can be imported without replacement.

## Release ownership

This repository owns only its downstream fork releases. Upstream releases and registry packages remain owned by their respective maintainers. The downstream release is not registered in a public provider registry and requires an explicitly configured, checksum-verified filesystem mirror. Report downstream issues in this repository.

## Requirements

-	[Terraform](https://www.terraform.io/downloads.html) >= 0.13.x (or [OpenTofu](https://opentofu.org))
-	[Go](https://golang.org/doc/install) >= 1.25 (source minimum; release builds use Go 1.27.2)

## Upgrading the provider

The Google Workspace provider doesn't upgrade automatically once you've started using it. After a new release you can run

```bash
terraform init -upgrade
```

to upgrade to the latest stable version of the Google Workspace provider. See the [Terraform website](https://www.terraform.io/docs/configuration/providers.html#provider-versions)
for more information on provider upgrades, and how to set version constraints on your provider.

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command or `make build`:
```sh
$ make build
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using The provider

See the [Google Workspace Provider documentation](https://registry.terraform.io/providers/SamuZad/googleworkspace/latest/docs) to get started using the
Google Workspace provider.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).
You can use [goenv](https://github.com/syndbg/goenv) to manage your Go version.
To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```sh
$ make testacc
```

For guidance on common development practices such as testing changes, see the [contribution guidelines](https://github.com/SamuZad/terraform-provider-googleworkspace/blob/main/.github/CONTRIBUTING.md).
If you have other development questions we don't cover, please file an issue!

## Special Recognition

* [Chase](https://github.com/DeviaVir) - for the excellent work creating the `DeviaVir/terraform-provider-gsuite` provider, the inspiration for this project.

## General Feedback
* How can we best support you ? - [feedback](https://forms.gle/XeqgPiFTtdevcRiu8)
## Focused fork release

The `0.12.1-fork.1` release uses upstream `SamuZad/googleworkspace` v0.12.0
(commit `86f6d55fceb122c1d6b3c69d10b6753fdc49f877`) with a role-assignment
refresh correction. It restores the API assignee type in state, normalizes USER
and GROUP values, and rejects missing or unsupported types. Existing imports
therefore converge without proposing assignment replacement.

Tests use synthetic identifiers and an in-memory HTTP transport. This release
introduces no tenant-specific configuration, credentials or access changes.
Builds for Linux amd64/arm64 and macOS arm64 are distributed as versioned ZIP
archives with SHA-256 checksums. The release has no registry entry or GPG
signature; consumers must verify the published archive digests and provider lock
hashes through an explicitly configured filesystem mirror. The inherited tag-triggered release workflow remains disabled until its signing
credentials and release lane are configured; this release uses the explicit
manual build and upload path below.

Build release archives from a clean committed checkout with Go 1.27.2:

```sh
python3 scripts/test-build-fork-release.py
go test ./internal/provider -run '^TestRoleAssignmentRefresh' -count=1
python3 scripts/build-fork-release.py --version 0.12.1-fork.1 --output /tmp/provider-release
```

The build helper never publishes. Review the source manifest and SHA-256 file
before uploading assets to an immutable versioned release. A later release must
use a new version and newly reviewed checksums; never replace published assets.
