package memory

import "encoding/json"

// PhotoRef 指向原始记录里的一张照片。记忆本身不存图，原图在原始记录的附件里（OSS / 本机目录）。
type PhotoRef struct {
	Note  string `json:"note"`
	Index int    `json:"index"`
}

// Photos 读出记忆 attrs.photos 里的照片引用。
func (m Memory) Photos() []PhotoRef {
	var attrs struct {
		Photos []PhotoRef `json:"photos"`
	}
	if len(m.Attrs) == 0 || json.Unmarshal(m.Attrs, &attrs) != nil {
		return nil
	}
	return attrs.Photos
}

// MergeAttrs 把 extra 的字段并进 base：照片引用取并集，其它非空字段以 extra 为准。
func MergeAttrs(base, extra json.RawMessage) json.RawMessage {
	merged := map[string]any{}
	_ = json.Unmarshal(base, &merged)
	var add map[string]any
	if json.Unmarshal(extra, &add) != nil {
		return base
	}
	var photos []PhotoRef
	for _, source := range []json.RawMessage{base, extra} {
		var attrs struct {
			Photos []PhotoRef `json:"photos"`
		}
		_ = json.Unmarshal(source, &attrs)
		for _, ref := range attrs.Photos {
			duplicate := false
			for _, existing := range photos {
				if existing == ref {
					duplicate = true
					break
				}
			}
			if !duplicate {
				photos = append(photos, ref)
			}
		}
	}
	for key, value := range add {
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		merged[key] = value
	}
	if len(photos) > 0 {
		merged["photos"] = photos
	}
	data, _ := json.Marshal(merged)
	return data
}
