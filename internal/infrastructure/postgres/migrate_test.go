package postgres

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 用量事实表与核算表持续增长；1.2.0 之后的迁移在这些表上建索引时必须使用 CONCURRENTLY，
// 并放在 NO TRANSACTION 迁移中，避免建索引期间阻塞网关写入用量和核算任务。
func TestMigrationsBuildLargeTableIndexesConcurrently(t *testing.T) {
	const firstCheckedVersion = 56
	largeTables := map[string]bool{"usage_record": true, "usage_rating": true}
	createIndex := regexp.MustCompile(`(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?\w+\s+ON\s+(?:ONLY\s+)?(\w+)`)
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		name := strings.TrimPrefix(file, "migrations/")
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			t.Fatalf("migration %s has no numeric version", name)
		}
		if version < firstCheckedVersion {
			continue
		}
		content, err := fs.ReadFile(migrations, file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		noTransaction := regexp.MustCompile(`(?m)^--\s*\+goose\s+NO\s+TRANSACTION\s*$`).MatchString(text)
		for _, match := range createIndex.FindAllStringSubmatch(text, -1) {
			concurrently, table := match[1] != "", strings.ToLower(match[2])
			if !largeTables[table] {
				continue
			}
			checked++
			if !concurrently || !noTransaction {
				t.Errorf("%s builds an index on %s with concurrently=%t no_transaction=%t; want both",
					name, table, concurrently, noTransaction)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no index on usage_record or usage_rating was checked")
	}
}
