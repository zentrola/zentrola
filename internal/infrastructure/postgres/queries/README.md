此目录存放 sqlc 查询文件，包括安全、管理、目录、网关、用量与授权读写。

sqlc 生成代码放入同层的 `dbgen/`，由 Repository 实现调用，不能进入 Domain，也不能直接序列化为 API 响应。

当前固定使用 sqlc 1.30.0：在项目根目录运行 `sqlc generate`，或者使用 `sqlc/sqlc:1.30.0` 容器。
数据库数字、时间和可空字段保留 pgx 原生准确语义；NUMERIC 不映射为 float。
所有运行时关联校验与管理写入事务在对应 Application 用例实现时补齐，不为后续功能预建通用 CRUD。
