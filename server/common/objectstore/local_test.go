package objectstore

import "testing"

func TestLocalSaveLoadAndEscape(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("notes/n1/0.jpg", []byte("img")); err != nil {
		t.Fatal(err)
	}
	data, err := store.Load("notes/n1/0.jpg")
	if err != nil || string(data) != "img" {
		t.Fatalf("读回不一致: %q %v", data, err)
	}
	// ../ 会被钳制在根目录内，读不到根目录外的文件
	if _, err := store.Load("../../etc/passwd"); err == nil {
		t.Fatal("不应读到根目录外的文件")
	}
}
