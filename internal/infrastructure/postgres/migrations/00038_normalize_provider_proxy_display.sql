-- +goose Up
-- 旧版本通过 net/url 生成脱敏代理地址，星号会被编码为 %2A。
-- 这里只修正不含真实凭据的展示列，代理密文保持不变。
UPDATE provider
SET proxy_url_display = replace(proxy_url_display, '%2A', '*')
WHERE strpos(proxy_url_display, '%2A') > 0;

-- +goose Down
UPDATE provider
SET proxy_url_display = replace(proxy_url_display, '*', '%2A')
WHERE strpos(proxy_url_display, '*') > 0;
