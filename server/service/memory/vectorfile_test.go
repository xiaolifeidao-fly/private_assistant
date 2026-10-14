package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileVectorStoreReadWrite(t *testing.T) {
	ctx := context.Background()
	store := FileVectorStore{Dir: t.TempDir()}
	if empty, _ := store.Empty(ctx, "default", "embo-01"); !empty {
		t.Fatal("新目录应为空")
	}
	vectors, cursor, full, err := store.Read(ctx, "default", "embo-01", nil)
	if err != nil || !full || len(vectors) != 0 {
		t.Fatalf("文件不存在时返回空的全量: %v %v", vectors, err)
	}
	if err := store.Write(ctx, "default", "embo-01", map[string][]float32{"mem_a": {0.6, 0.8}, "mem_b": {1, 0}}); err != nil {
		t.Fatal(err)
	}
	vectors, cursor, full, err = store.Read(ctx, "default", "embo-01", cursor)
	if err != nil || !full || len(vectors) != 2 || vectors["mem_a"][1] != 0.8 {
		t.Fatalf("第一次读应是全量: %v %v %v", vectors, full, err)
	}

	// 另一个进程（另一个 store 实例）写入，增量读只拿到新的
	other := FileVectorStore{Dir: store.Dir}
	if err := other.Write(ctx, "default", "embo-01", map[string][]float32{"mem_c": {0, 1}, "mem_a": {1, 0}}); err != nil {
		t.Fatal(err)
	}
	vectors, cursor, full, err = store.Read(ctx, "default", "embo-01", cursor)
	if err != nil || full || len(vectors) != 2 || vectors["mem_a"][0] != 1 {
		t.Fatalf("增量读只拿新写的，后写覆盖先写: %v %v %v", vectors, full, err)
	}

	// 模型名带斜杠也能存
	if err := store.Write(ctx, "default", "BAAI/bge-m3", map[string][]float32{"mem_a": {1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "default", "BAAI_bge-m3.vec")); err != nil {
		t.Fatalf("模型名应转成安全文件名: %v", err)
	}
}

func TestFileVectorStoreSkipsTornTailAndMaintains(t *testing.T) {
	ctx := context.Background()
	store := FileVectorStore{Dir: t.TempDir()}
	_ = store.Write(ctx, "default", "m", map[string][]float32{"mem_a": {1, 0}})
	path := store.path("default", "m")
	good, _ := os.Stat(path)
	// 模拟写到一半崩溃：追加半条记录
	file, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	_, _ = file.Write([]byte{200, 0, 0, 0, 5, 0, 'm'})
	file.Close()
	vectors, _, _, err := store.Read(ctx, "default", "m", nil)
	if err != nil || len(vectors) != 1 {
		t.Fatalf("半条记录要跳过: %v %v", vectors, err)
	}
	if err := store.Maintain("default", "m"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Size() != good.Size() {
		t.Fatalf("Maintain 应截掉半条记录: %d != %d", info.Size(), good.Size())
	}

	// 同一条记忆反复重算：重复记录多了压缩重写；读方发现换了文件，整体重载
	_, cursor, _, _ := store.Read(ctx, "default", "m", nil)
	for i := 0; i < 70; i++ {
		_ = store.Write(ctx, "default", "m", map[string][]float32{"mem_a": {float32(i), 1}})
	}
	_ = store.Write(ctx, "default", "m", map[string][]float32{"mem_b": {0, 1}})
	before, _ := os.Stat(path)
	if err := store.Maintain("default", "m"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if after.Size() >= before.Size() {
		t.Fatalf("应压缩: %d → %d", before.Size(), after.Size())
	}
	vectors, _, full, err := store.Read(ctx, "default", "m", cursor)
	if err != nil || !full || len(vectors) != 2 || vectors["mem_a"][0] != 69 {
		t.Fatalf("压缩后要整体重载并保留最新的: %v full=%v %v", vectors, full, err)
	}
}

func TestVectorIndexSyncsFromStore(t *testing.T) {
	ctx := context.Background()
	store := FileVectorStore{Dir: t.TempDir()}
	_ = store.Write(ctx, "f", "m", map[string][]float32{"mem_a": {1, 0}})
	index := newVectorIndex()
	if err := index.ensure(ctx, store, "f", "m", false); err != nil {
		t.Fatal(err)
	}
	_ = store.Write(ctx, "f", "m", map[string][]float32{"mem_b": {0, 1}})
	_ = index.ensure(ctx, store, "f", "m", false)
	if len(index.has("f", "m")) != 1 {
		t.Fatal("30 秒内不重复读")
	}
	_ = index.ensure(ctx, store, "f", "m", true)
	if has := index.has("f", "m"); !has["mem_a"] || !has["mem_b"] {
		t.Fatalf("强制同步应读到另一个进程写的: %v", has)
	}
	_ = index.ensure(ctx, store, "f", "other", false)
	if len(index.has("f", "other")) != 0 || len(index.has("f", "m")) != 0 {
		t.Fatal("换模型整体重载，不混用")
	}
}
