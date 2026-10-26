# Release Process / 发布流程

Only maintainers should publish a release. Version tags and published artifacts are immutable; a correction after release uses a new patch version.

仅维护者可以发布版本。版本 tag 和已发布产物不可覆盖；发布后的修复必须递增 patch 版本。

## 1. Prepare / 准备

1. Update `APP_VERSION` in `.env.example`, `ARG APP_VERSION` in `Dockerfile`, `web/package.json`, `web/package-lock.json`, `CHANGELOG.md`, and user documentation to the same semantic version. `.env.example` is the single source of the release version; it is embedded into the backend at build time and is not read at runtime.
2. Run `scripts/Sync-ThirdPartyLicenses.ps1` and review all license changes.
3. Confirm migrations, generated code, compatibility notes, and required configuration changes.
4. Ensure the working tree is clean and the release commit is pushed.

## 2. Verify / 验证

Run:

```shell
go test ./...
go vet ./...
go build ./cmd/server ./cmd/web
./scripts/Verify-Version.ps1
cd web
npm ci
npm run format:check
npm run build
npm test
```

Run `sqlc generate` with sqlc 1.30.0 and confirm there is no generated diff. Verify a release bundle contains `.env.example`, `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and `third_party_licenses/`, and that the Admin Web sidebar shows the release version.

使用 sqlc 1.30.0 执行 `sqlc generate` 并确认没有生成差异。确认发布包包含 `.env.example`、`LICENSE`、`NOTICE`、`THIRD_PARTY_NOTICES.md` 和 `third_party_licenses/`，且管理后台侧边栏显示发布版本号。

## 3. Tag and release / 创建标签与发布

Create an annotated tag `v<version>` whose version matches `APP_VERSION` in `.env.example`. The release workflow only runs for tags starting with `v`.

创建与 `.env.example` 中 `APP_VERSION` 一致的附注标签 `v<版本号>`；发布工作流只响应以 `v` 开头的标签。

```powershell
$version = (Select-String -Path .env.example -Pattern '^APP_VERSION=(.+)$').Matches[0].Groups[1].Value.Trim()
git tag -a "v$version" -m "发布 $version"
git push origin "v$version"
```

The release workflow builds platform bundles, checksums, and an SBOM, then creates the GitHub Release. Review the generated release notes before announcing it.

发布工作流会构建各平台发布包、校验和及 SBOM，并创建 GitHub Release；正式公告前应检查自动生成的发布说明。

Container images must include OCI `version`, `revision`, and `created` labels. Record the immutable image digest in the release notes. Never move an existing release tag or overwrite an already published version tag; publish `1.0.1`, `1.0.2`, and so on.

容器镜像必须包含 OCI `version`、`revision` 和 `created` 标签，并在发布说明中记录不可变 digest。不得移动已有发布 tag，也不得覆盖已经发布的版本镜像；后续修复应发布 `1.0.1`、`1.0.2` 等新版本。
