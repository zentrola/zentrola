# 安全策略

[English](SECURITY.md) | [简体中文](SECURITY.zh-CN.md)

Zentrola 会处理 Provider Credential、Virtual Key、管理员会话、prompt 和模型输出。发现疑似漏洞或秘密意外泄露时，必须按照敏感事件处理。

## 支持版本

Zentrola 暂未公布稳定的版本支持矩阵。安全修复以最新发布版本为目标。报告可能已经修复的问题前，请先升级到最新版本进行确认。

## 报告安全漏洞

不要通过公开 Issue 提交漏洞细节、Credential、利用步骤、私有日志、prompt 或模型响应。

如果 `zentrola/zentrola` 已启用 GitHub Private Vulnerability Reporting，请优先通过该功能报告；否则发送邮件至维护者 `longjianghu1982@gmail.com`。首次邮件只发送建立安全沟通所必需的信息，不要附带有效凭据或敏感用户数据。

有效报告应包括：

- 受影响版本或 commit；
- 受影响组件和配置；
- 使用合成数据描述的影响和复现步骤；
- 是否已经观察到实际利用；
- 已知的缓解建议。

必须删除或替换所有真实 Provider Credential、Virtual Key、JWT、Cookie、数据库连接信息、prompt、模型响应和个人信息。

## 响应说明

项目目前不承诺正式响应 SLA。维护者应确认有效的私密报告、协同验证和修复、在修复发布前避免过早披露，并在报告者希望且条件合适时给予致谢。

## 运维安全

- Admin Web 和 Gateway 必须使用 HTTPS。
- PostgreSQL 和 Redis 应部署在私有网络。
- Admin JWT 签名、PostgreSQL、Redis 和 Provider 认证应使用相互独立的秘密。
- PostgreSQL 必须与加密 Provider Credential 时使用的原 Master Key 一起备份。
- 生产环境不得启用 `APP_ENV=dev` 的请求和响应正文诊断。
- 暴露或不再需要的 Virtual Key 应立即撤销。
- 任何可能出现在日志、截图、Issue 或聊天记录中的秘密都必须轮换；事后涂黑不能恢复秘密的安全性。

部署细节见[部署与运维](docs/operations.zh-CN.md)。
