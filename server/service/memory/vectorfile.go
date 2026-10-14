package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// FileVectorStore 把向量存在服务器本地文件里：{dir}/{家庭}/{模型}.vec，每个模型一个文件。
//
// 文件是只追加的日志：开头 8 字节魔数，之后一条条记录
//
//	uint32 记录长度 | uint16 key 长度 | key | uint32 维度 | 维度×float32（小端） | uint32 CRC32
//
// 同一条记忆后写的覆盖先写的。两个 API 进程共用一个目录：写用 flock 排他锁、读用共享锁；
// 读方记下读到的偏移量，下次只读新追加的部分。进程中途崩溃留下的半条记录读时跳过，
// 启动时 Maintain 截掉；重复记录太多时 Maintain 把文件压缩重写（换新文件，读方发现后整体重载）。
type FileVectorStore struct{ Dir string }

var vectorFileMagic = []byte("PAVEC01\n")

type fileCursor struct {
	info   os.FileInfo
	offset int64
}

func (store FileVectorStore) path(familyKey, model string) string {
	return filepath.Join(store.Dir, safeName(familyKey), safeName(model)+".vec")
}

// safeName 把模型名变成安全的文件名（模型名里可能有 / : 等字符，如 BAAI/bge-m3）。
func safeName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '-', char == '_', char == '.':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 || strings.Trim(builder.String(), ".") == "" {
		return "_"
	}
	return builder.String()
}

func (store FileVectorStore) Read(_ context.Context, familyKey, model string, cursor any) (map[string][]float32, any, bool, error) {
	file, err := os.Open(store.path(familyKey, model))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]float32{}, nil, true, nil
	}
	if err != nil {
		return nil, cursor, false, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return nil, cursor, false, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	info, err := file.Stat()
	if err != nil {
		return nil, cursor, false, err
	}
	start, full := int64(len(vectorFileMagic)), true
	if previous, ok := cursor.(fileCursor); ok && os.SameFile(previous.info, info) && info.Size() >= previous.offset {
		start, full = previous.offset, false
	}
	vectors, end, err := readRecords(file, start, info.Size())
	if err != nil {
		return nil, cursor, false, err
	}
	return vectors, fileCursor{info: info, offset: end}, full, nil
}

func (store FileVectorStore) Write(_ context.Context, familyKey, model string, vectors map[string][]float32) error {
	if len(vectors) == 0 {
		return nil
	}
	var buffer bytes.Buffer
	for memoryKey, vector := range vectors {
		appendRecord(&buffer, memoryKey, vector)
	}
	file, err := store.lock(familyKey, model, os.O_RDWR|os.O_CREATE|os.O_APPEND)
	if err != nil {
		return err
	}
	defer unlockClose(file)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		if _, err := file.Write(vectorFileMagic); err != nil {
			return err
		}
	}
	// 一次写入整批，加上排他锁，另一个进程读不到半批
	if _, err := file.Write(buffer.Bytes()); err != nil {
		return err
	}
	return file.Sync()
}

func (store FileVectorStore) Empty(_ context.Context, familyKey, model string) (bool, error) {
	info, err := os.Stat(store.path(familyKey, model))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return info.Size() <= int64(len(vectorFileMagic)), nil
}

// Maintain 截掉崩溃留下的半条记录；重复记录（同一条记忆被重算过）超过一半时压缩重写。启动时调用。
func (store FileVectorStore) Maintain(familyKey, model string) error {
	file, err := store.lock(familyKey, model, os.O_RDWR)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unlockClose(file)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < int64(len(vectorFileMagic)) {
		return file.Truncate(0)
	}
	vectors, end, records, err := scanRecords(file, info.Size())
	if err != nil {
		return err
	}
	if end < info.Size() {
		if err := file.Truncate(end); err != nil {
			return err
		}
	}
	if records < 64 || records < 2*len(vectors) {
		return nil
	}
	// 写到临时文件再改名：正在读旧文件的进程不受影响，下次读时发现换了文件就整体重载
	path := store.path(familyKey, model)
	temp := path + ".tmp"
	var buffer bytes.Buffer
	buffer.Write(vectorFileMagic)
	for memoryKey, vector := range vectors {
		appendRecord(&buffer, memoryKey, vector)
	}
	if err := os.WriteFile(temp, buffer.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// lock 打开文件并加排他锁。加锁期间文件可能被 Maintain 换掉（改名），那样锁住的是旧文件，要重开。
func (store FileVectorStore) lock(familyKey, model string, flag int) (*os.File, error) {
	path := store.path(familyKey, model)
	if flag&os.O_CREATE != 0 {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 5; attempt++ {
		file, err := os.OpenFile(path, flag, 0o600)
		if err != nil {
			return nil, err
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
			file.Close()
			return nil, err
		}
		opened, err1 := file.Stat()
		current, err2 := os.Stat(path)
		if err1 == nil && err2 == nil && os.SameFile(opened, current) {
			return file, nil
		}
		unlockClose(file)
	}
	return nil, fmt.Errorf("向量文件 %s 一直在被替换，稍后再试", path)
}

func unlockClose(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func appendRecord(buffer *bytes.Buffer, memoryKey string, vector []float32) {
	body := make([]byte, 0, 2+len(memoryKey)+4+4*len(vector))
	body = binary.LittleEndian.AppendUint16(body, uint16(len(memoryKey)))
	body = append(body, memoryKey...)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(vector)))
	for _, value := range vector {
		body = binary.LittleEndian.AppendUint32(body, math.Float32bits(value))
	}
	_ = binary.Write(buffer, binary.LittleEndian, uint32(len(body)+4))
	buffer.Write(body)
	_ = binary.Write(buffer, binary.LittleEndian, crc32.ChecksumIEEE(body))
}

func readRecords(file *os.File, start, size int64) (map[string][]float32, int64, error) {
	vectors := map[string][]float32{}
	if start == int64(len(vectorFileMagic)) {
		magic := make([]byte, len(vectorFileMagic))
		if _, err := file.ReadAt(magic, 0); err != nil || !bytes.Equal(magic, vectorFileMagic) {
			return vectors, start, fmt.Errorf("不是向量文件（开头不对）: %s", file.Name())
		}
	}
	if size <= start {
		return vectors, start, nil
	}
	data := make([]byte, size-start)
	if _, err := file.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
		return nil, start, err
	}
	offset := 0
	for offset+4 <= len(data) {
		length := int(binary.LittleEndian.Uint32(data[offset:]))
		if length < 2+4+4 || offset+4+length > len(data) {
			break // 半条记录：崩溃留下的，或者另一个进程正在写（读到的长度不完整）
		}
		record := data[offset+4 : offset+4+length]
		body, checksum := record[:length-4], binary.LittleEndian.Uint32(record[length-4:])
		if crc32.ChecksumIEEE(body) != checksum {
			break
		}
		keyLength := int(binary.LittleEndian.Uint16(body))
		if 2+keyLength+4 > len(body) {
			break
		}
		memoryKey := string(body[2 : 2+keyLength])
		dim := int(binary.LittleEndian.Uint32(body[2+keyLength:]))
		raw := body[2+keyLength+4:]
		if len(raw) != dim*4 {
			break
		}
		vectors[memoryKey] = decodeVector(raw)
		offset += 4 + length
	}
	return vectors, start + int64(offset), nil
}

// scanRecords 读全部有效记录，返回去重后的向量、有效部分的结尾和记录总数。
func scanRecords(file *os.File, size int64) (map[string][]float32, int64, int, error) {
	vectors, end, err := readRecords(file, int64(len(vectorFileMagic)), size)
	if err != nil {
		return nil, 0, 0, err
	}
	// 记录总数：按长度字段数一遍（不解码向量）
	records := 0
	header := make([]byte, 4)
	for offset := int64(len(vectorFileMagic)); offset < end; records++ {
		if _, err := file.ReadAt(header, offset); err != nil {
			return nil, 0, 0, err
		}
		offset += 4 + int64(binary.LittleEndian.Uint32(header))
	}
	return vectors, end, records, nil
}
