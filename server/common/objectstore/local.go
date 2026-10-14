// Package objectstore 存原始记录的附件（截图等）。现在是本机目录；
// 以后要上对象存储，换掉实现即可，调用方只认 Save / Load。
package objectstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Local struct{ root string }

// NewLocal 返回以 root 为根目录的存储；目录不存在时创建。
func NewLocal(root string) (*Local, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &Local{root: absolute}, nil
}

func (l *Local) Save(path string, data []byte) error {
	target, err := l.resolve(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}

func (l *Local) Load(path string) ([]byte, error) {
	target, err := l.resolve(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

// resolve 拒绝跳出根目录的路径。
func (l *Local) resolve(path string) (string, error) {
	target := filepath.Join(l.root, filepath.Clean("/"+path))
	if !strings.HasPrefix(target, l.root+string(filepath.Separator)) {
		return "", errors.New("非法的存储路径")
	}
	return target, nil
}
