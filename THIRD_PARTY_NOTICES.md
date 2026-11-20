# Third-Party Notices

Zentrola incorporates third-party software. The corresponding license texts are distributed in the `third_party_licenses` directory and remain the property of their respective copyright holders. Nothing in the Zentrola license changes the terms of those components.

The Go inventory is derived from the runtime dependency graph of `cmd/server` and `cmd/web`. The npm inventory contains production packages from `web/package-lock.json`. Run `scripts/Sync-ThirdPartyLicenses.ps1` after changing either dependency graph, then review and commit the result.

Machine-readable component and license metadata is produced by the repository's SBOM workflow for every release tag. This notice and the accompanying license texts must be included in standalone release bundles and container images.
