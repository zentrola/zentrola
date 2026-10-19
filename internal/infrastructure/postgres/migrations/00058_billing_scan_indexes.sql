-- +goose NO TRANSACTION

-- usage_record 与 usage_rating 持续增长，普通 CREATE INDEX 会在建索引期间阻塞网关写入用量与核算任务；
-- 本迁移在事务外以 CONCURRENTLY 方式建索引，只需等待已开始的写事务结束，不阻塞新的写入。
-- 并发建索引失败会遗留 INVALID 索引，且本迁移不会记录版本；因此每个索引先 DROP ... IF EXISTS 再创建，
-- 重新执行时可以清理遗留索引。依赖这些索引的查询与触发器在索引就绪前仍然正确，只是更慢。

-- +goose Up

-- 核算任务按用量 ID 顺序反连接查找从未核算的可计费用量；部分索引只覆盖可计费的成功调用，
-- 并包含凭据、模型与调用时间，使反连接、凭据类型与价格存在性过滤可以只读索引完成，
-- 只为最终入选的一页记录回表。
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_record_rating_candidate;
CREATE INDEX CONCURRENTLY ix_usage_record_rating_candidate
    ON usage_record (id) INCLUDE (provider_credential_id, provider_model_id, started_at)
    WHERE status = 'SUCCESS'
      AND input_tokens IS NOT NULL
      AND cached_input_tokens IS NOT NULL
      AND output_tokens IS NOT NULL
      AND cached_input_tokens <= input_tokens;

-- 价格金额纠错后只需重算价格快照与当前价格不一致的核算记录；按价格版本和价格快照（含缓存写入单价）
-- 建索引，让重算扫描通过范围条件跳过快照仍然一致的记录，同时加速引用保护触发器的 model_price_id 查找。
-- 缓存写入单价 NULL 统一映射为 -1（单价不可能为负），表达式必须与重算扫描查询保持一致。
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_rating_price_snapshot;
CREATE INDEX CONCURRENTLY ix_usage_rating_price_snapshot
    ON usage_rating (model_price_id, input_price, cached_input_price, output_price, COALESCE(cache_creation_input_price, -1));

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_rating_price_snapshot;
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_record_rating_candidate;
