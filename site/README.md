# Zentrola 项目网站

`site/` 是 GitHub Pages 的静态页面源文件。中文首页位于 `index.html`，英文页面位于 `en/index.html`，共用 `assets/style.css`。

`.github/workflows/pages.yml` 在 `main` 分支的页面源文件或相关图片变更后发布网站。工作流将 `docs/assets/admin-console.png` 和 `web/public/favicon.svg` 复制到发布目录；不要在 `site/assets/` 保存另一份图片。

首次发布前，在仓库的 **Settings → Pages → Build and deployment** 中把 **Source** 设为 **GitHub Actions**。默认网址为 `https://zentrola.github.io/zentrola/`。本网站只介绍项目，管理界面和 Go Backend 仍按正式部署文档运行。
