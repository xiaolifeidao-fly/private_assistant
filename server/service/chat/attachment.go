package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"service/bot"
	"service/intake"
)

// 带照片的消息（所有对话都支持，拍照或粘贴）：照片先存成原始记录（channel=capture），这一轮当场整理成记忆，
// 整理结果以 <attachment> 块交给模型；模型只看文字，不在对话历史里放图片（省 token，历史也稳定）。
// 回答里引用到带照片的记忆时，把照片引用写进回答的 meta，手机端显示原图。

const maxAnswerPhotos = 9

type attachmentMeta struct {
	NoteKey  string   `json:"noteKey"`
	Photos   int      `json:"photos"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary,omitempty"`
	Memories []string `json:"memories,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type photoMeta struct {
	MemoryKey string `json:"memoryKey"`
	Title     string `json:"title"`
	NoteKey   string `json:"noteKey"`
	Index     int    `json:"index"`
}

func (s *Service) ingestAttachment(ctx context.Context, familyKey string, item bot.Bot, noteKey string) (*attachmentMeta, string, error) {
	if s.notes == nil {
		return nil, "", fmt.Errorf("这个服务没有开启照片整理")
	}
	if _, err := s.notes.GetNote(ctx, familyKey, noteKey); err != nil {
		return nil, "", fmt.Errorf("照片记录 %s 不存在", noteKey)
	}
	note, err := s.notes.ProcessNow(ctx, familyKey, noteKey)
	if err != nil {
		return nil, "", err
	}
	meta := &attachmentMeta{NoteKey: noteKey, Photos: len(note.Attachments), Status: note.ExtractStatus, Error: note.ExtractError}
	var builder strings.Builder
	fmt.Fprintf(&builder, "<attachment note=%q photos=\"%d\">\n", noteKey, meta.Photos)
	switch {
	case note.ExtractStatus == intake.StatusDone && note.Result != nil:
		meta.Summary = note.Result.Summary
		meta.Memories = append(meta.Memories, note.Result.Added...)
		meta.Memories = append(meta.Memories, note.Result.Pending...)
		meta.Memories = append(meta.Memories, note.Result.Updated...)
		meta.Memories = append(meta.Memories, note.Result.Duplicates...)
		fmt.Fprintf(&builder, "我刚发了 %d 张照片（截图或拍照），下面是我说的话。系统已经看过照片，整理结果：%s\n", meta.Photos, note.Result.Summary)
		groups := []struct {
			label string
			keys  []string
		}{{"新记下", note.Result.Added}, {"待我确认（没把握或和旧记忆冲突）", note.Result.Pending}, {"更新了已有记忆（如东西换了地方）", note.Result.Updated}, {"以前已经记过", note.Result.Duplicates}}
		dir, _ := s.loadDirectory(ctx, familyKey)
		for _, group := range groups {
			for _, key := range group.keys {
				if item, err := s.memory.Get(ctx, familyKey, key); err == nil {
					fmt.Fprintf(&builder, "- %s：%s\n", group.label, formatMemory(dir, item))
				}
			}
		}
		if len(meta.Memories) == 0 {
			builder.WriteString("没有整理出值得记的内容。\n")
		}
		if len(note.Result.UnknownPersons) > 0 {
			fmt.Fprintf(&builder, "认不出的人：%s\n", strings.Join(note.Result.UnknownPersons, "、"))
		}
		builder.WriteString(attachmentAsk(item))
	case note.ExtractStatus == intake.StatusFailed || note.ExtractError != "":
		fmt.Fprintf(&builder, "我刚发了 %d 张照片，照片已经保存，但没看成：%s。告诉我照片存好了、稍后会自动重试，不要编造照片内容。\n", meta.Photos, note.ExtractError)
	default:
		fmt.Fprintf(&builder, "我刚发了 %d 张照片，照片已经保存，还在后台整理。告诉我整理好后可以在「速记」里查看，不要编造照片内容。\n", meta.Photos)
	}
	builder.WriteString("</attachment>")
	return meta, builder.String(), nil
}

// attachmentAsk 是整理结果之后给模型的要求，按对话的角色分：
// 万能通报记下了什么、问位置；提醒看照片里有没有要提醒的时间；分身以 TA 的身份回应；其余按我的话回答。
func attachmentAsk(item bot.Bot) string {
	const noRepeat = "照片里的内容已经记下了，不要再用 remember 重复记。只依据上面的整理结果说照片里有什么，不要编造。\n"
	switch {
	case item.Kind == bot.KindPersona:
		return "按你的身份回应我发的照片和我说的话；整理结果里认不出或没看到的，就当没看到，不要编造。\n"
	case item.BotKey == "recall":
		return "请简短告诉我记下了什么（东西、用途、位置），引用记忆编号；位置没说清的，问我放在哪。" + noRepeat
	case item.BotKey == "remind":
		return "看照片里有没有要我去做、有时间点的事（通知、日程、缴费单、聊天截图里的约定），按平常的规则整理成提醒；时间不清楚的问我。" + noRepeat
	default:
		return "结合整理结果回答我说的话；我只发了照片没说什么的，简短告诉我记下了什么，引用记忆编号。" + noRepeat
	}
}

// citedPhotos 收集回答里引用到的记忆所附的照片，按引用顺序，最多 9 张。
func (s *Service) citedPhotos(ctx context.Context, familyKey string, cited []string) []photoMeta {
	var photos []photoMeta
	for _, key := range cited {
		item, err := s.memory.Get(ctx, familyKey, key)
		if err != nil {
			continue
		}
		for _, ref := range item.Photos() {
			if len(photos) >= maxAnswerPhotos {
				return photos
			}
			photos = append(photos, photoMeta{MemoryKey: key, Title: item.Title, NoteKey: ref.Note, Index: ref.Index})
		}
	}
	return photos
}

func hasAttachment(row messageRow) bool {
	if len(row.Meta) == 0 {
		return false
	}
	var meta turnMeta
	return json.Unmarshal(row.Meta, &meta) == nil && meta.Attachment != nil
}

// attrsWithoutPhotos 把 attrs 转成给模型看的文本，去掉照片引用（模型用不上，徒占 token）。
func attrsWithoutPhotos(attrs json.RawMessage) string {
	if len(attrs) == 0 || string(attrs) == "null" || string(attrs) == "{}" {
		return ""
	}
	var fields map[string]any
	if json.Unmarshal(attrs, &fields) != nil {
		return string(attrs)
	}
	if _, ok := fields["photos"]; !ok {
		return string(attrs)
	}
	delete(fields, "photos")
	if len(fields) == 0 {
		return ""
	}
	data, _ := json.Marshal(fields)
	return string(data)
}
