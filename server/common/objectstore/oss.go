package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

// OSSConfig 是阿里云 OSS 的连接配置。桶必须是私有的：图片只经 API 鉴权后转发给手机，不直接暴露。
type OSSConfig struct {
	Region          string // 如 cn-shanghai
	Endpoint        string // 可选，如 oss-cn-shanghai-internal.aliyuncs.com（同地域 ECS 走内网免流量费）
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	Prefix          string // 对象键前缀，如 private_assistant/
}

type OSS struct {
	client *oss.Client
	bucket string
	prefix string
}

const ossTimeout = 60 * time.Second

func NewOSS(config OSSConfig) (*OSS, error) {
	if config.Region == "" || config.Bucket == "" || config.AccessKeyID == "" || config.AccessKeySecret == "" {
		return nil, errors.New("OSS 配置不完整：region、bucket、access_key_id、access_key_secret 都要填")
	}
	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.AccessKeySecret)).
		WithRegion(config.Region)
	if config.Endpoint != "" {
		cfg = cfg.WithEndpoint(config.Endpoint)
	}
	prefix := strings.Trim(config.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &OSS{client: oss.NewClient(cfg), bucket: config.Bucket, prefix: prefix}, nil
}

func (o *OSS) key(path string) (string, error) {
	clean := strings.TrimPrefix(path, "/")
	if clean == "" || strings.Contains(clean, "..") {
		return "", errors.New("非法的存储路径")
	}
	return o.prefix + clean, nil
}

func (o *OSS) Save(path string, data []byte) error {
	key, err := o.key(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ossTimeout)
	defer cancel()
	_, err = o.client.PutObject(ctx, &oss.PutObjectRequest{Bucket: oss.Ptr(o.bucket), Key: oss.Ptr(key), Body: bytes.NewReader(data)})
	if err != nil {
		return fmt.Errorf("上传 OSS 失败: %w", err)
	}
	return nil
}

func (o *OSS) Load(path string) ([]byte, error) {
	key, err := o.key(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ossTimeout)
	defer cancel()
	result, err := o.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(o.bucket), Key: oss.Ptr(key)})
	if err != nil {
		var serviceErr *oss.ServiceError
		if errors.As(err, &serviceErr) && serviceErr.StatusCode == 404 {
			return nil, ErrNotExist
		}
		return nil, fmt.Errorf("读取 OSS 失败: %w", err)
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

// ErrNotExist 表示对象不存在。
var ErrNotExist = errors.New("对象不存在")

// Store 是附件存储的最小能力。
type Store interface {
	Save(path string, data []byte) error
	Load(path string) ([]byte, error)
}

// Fallback 新文件只写主存储；读不到时再去旧存储找。
// 切到 OSS 以后，以前存在本机目录的截图不用搬也能照常查看。
type Fallback struct {
	Primary Store
	Legacy  Store
}

func (f Fallback) Save(path string, data []byte) error { return f.Primary.Save(path, data) }

func (f Fallback) Load(path string) ([]byte, error) {
	data, err := f.Primary.Load(path)
	if err == nil || f.Legacy == nil || !errors.Is(err, ErrNotExist) {
		return data, err
	}
	return f.Legacy.Load(path)
}
