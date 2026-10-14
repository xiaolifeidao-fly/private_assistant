package objectstore

import (
	"errors"
	"testing"
)

type memoryStore map[string][]byte

func (m memoryStore) Save(path string, data []byte) error { m[path] = data; return nil }
func (m memoryStore) Load(path string) ([]byte, error) {
	if data, ok := m[path]; ok {
		return data, nil
	}
	return nil, ErrNotExist
}

func TestFallbackReadsLegacy(t *testing.T) {
	primary, legacy := memoryStore{}, memoryStore{"old.jpg": []byte("old")}
	store := Fallback{Primary: primary, Legacy: legacy}
	if data, err := store.Load("old.jpg"); err != nil || string(data) != "old" {
		t.Fatalf("切换前的旧附件应能读到: %q %v", data, err)
	}
	_ = store.Save("new.jpg", []byte("new"))
	if _, ok := legacy["new.jpg"]; ok || string(primary["new.jpg"]) != "new" {
		t.Fatal("新附件只写主存储")
	}
	if _, err := store.Load("missing.jpg"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("都没有时返回不存在: %v", err)
	}
}
