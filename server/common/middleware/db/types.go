package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringList 映射 MySQL JSON 数组列。nil 存 NULL。
type StringList []string

// GormDataType 告诉 gorm 这是一个普通列：不声明的话，切片类型会被当成关联关系去解析，
// 报 unsupported data type: &[]。
func (StringList) GormDataType() string { return "json" }

func (list StringList) Value() (driver.Value, error) {
	if list == nil {
		return nil, nil
	}
	data, err := json.Marshal([]string(list))
	return string(data), err
}

func (list *StringList) Scan(value any) error {
	data, err := bytesOf(value)
	if err != nil || data == nil {
		*list = nil
		return err
	}
	return json.Unmarshal(data, (*[]string)(list))
}

// JSON 映射任意 JSON 列，保持原始字节。空值存 NULL。
type JSON json.RawMessage

func (JSON) GormDataType() string { return "json" }

func (raw JSON) Value() (driver.Value, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	return string(raw), nil
}

func (raw *JSON) Scan(value any) error {
	data, err := bytesOf(value)
	if err != nil || data == nil {
		*raw = nil
		return err
	}
	*raw = append((*raw)[:0], data...)
	return nil
}

func (raw JSON) MarshalJSON() ([]byte, error) {
	if len(raw) == 0 {
		return []byte("null"), nil
	}
	return raw, nil
}

func (raw *JSON) UnmarshalJSON(data []byte) error {
	*raw = append((*raw)[:0], data...)
	return nil
}

// MustJSON 把任意值编码成 JSON 列；编码失败返回 nil（存 NULL）。
func MustJSON(value any) JSON {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return JSON(data)
}

func bytesOf(value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		return nil, fmt.Errorf("不支持的 JSON 列类型 %T", value)
	}
}
