// Package keys 生成跨表引用用的字符串业务键（mem_xxx、conv_xxx …）。
//
// 自增 id 只在表内用；表与表之间一律用业务键引用，方便导入导出和日志排查。
package keys

import (
	"crypto/rand"
	"encoding/hex"
)

// New 返回 "<prefix>_<20 位十六进制>"，例如 mem_3f9a1c0b7e2d4a5c6b8e。
func New(prefix string) string {
	return prefix + "_" + Random(10)
}

// Random 返回 n 字节随机数的十六进制串。
func Random(n int) string {
	buffer := make([]byte, n)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
