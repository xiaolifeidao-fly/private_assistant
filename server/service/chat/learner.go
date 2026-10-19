package chat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"common/viewer"
	"service/bot"
	"service/intake"
	"service/llm"
)

// 对话自动学习（设计文档 06 · 2）：对话空闲一段时间后，把这段时间里我说的话整理成一条
// 原始记录（channel=chat），交给现有的抽取管线 —— 去重、冲突进待确认、按人刷新人设卡都复用那边。
//
// 只抽取我说的话。助理的话是建议、分身的话是模拟，都不是事实；助理的话只截一小段作为理解上下文。

const (
	learnerTick     = time.Minute
	learnerBatch    = 20
	learnMaxRunes   = 24000 // 原始记录上限 30000，留出说明文字的余量
	learnMinRunes   = 4     // 我说的话加起来太短（只有「好的」「嗯」）就不整理了
	learnerChannel  = "chat"
	learnerLockName = "pa_chat_learner"
)

const learnHeader = `以下是我和家庭助理（或家人分身）的一段对话记录，时间 %s。
请只抽取我陈述的事实：家人说过的话、发生的事、她们的喜好和变化、我答应的事。
不要抽取：我打算说但还没说出口的话、我在问的问题、助理的建议、分身的模拟回答。
【助理】后面的内容只用来理解上下文。

`

type IntakePort interface {
	CreateNote(ctx context.Context, familyKey string, input intake.NoteInput) (intake.Note, error)
}

// RunLearner 定时把空闲的对话交给记忆整理，直到 ctx 结束。两个进程都跑也没问题（GET_LOCK）。
func (s *Service) RunLearner(ctx context.Context, notes IntakePort) {
	ticker := time.NewTicker(learnerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		learning := s.settings.Learning(ctx)
		if !learning.ChatExtract {
			continue
		}
		s.learnIdle(ctx, notes, time.Duration(learning.ChatIdleMinutes)*time.Minute)
	}
}

func (s *Service) learnIdle(ctx context.Context, notes IntakePort, idle time.Duration) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", learnerLockName).Scan(&locked); err != nil || locked != 1 {
		return
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", learnerLockName)

	var rows []conversationRow
	err = s.db.WithContext(ctx).
		Where("last_message_at < ? AND EXISTS (SELECT 1 FROM pa_message m WHERE m.conversation_key = pa_conversation.conversation_key AND m.role = ? AND m.id > pa_conversation.extracted_message_id)",
			time.Now().Add(-idle), llm.RoleUser).
		Order("last_message_at").Limit(learnerBatch).Find(&rows).Error
	if err != nil {
		log.Printf("对话学习: %v", err)
		return
	}
	for _, conversation := range rows {
		if err := s.learnConversation(ctx, notes, conversation); err != nil {
			log.Printf("对话学习 %s: %v", conversation.ConversationKey, err)
		}
	}
}

// learnConversation 把一段对话里还没整理过的部分交给记忆整理，并推进进度。
func (s *Service) learnConversation(ctx context.Context, notes IntakePort, conversation conversationRow) error {
	var rows []messageRow
	if err := s.db.WithContext(ctx).Where("conversation_key = ? AND id > ? AND status = 'done' AND role IN ?",
		conversation.ConversationKey, conversation.ExtractedMessageID, []string{llm.RoleUser, llm.RoleAssistant}).
		Order("id").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	item, err := s.bots.Get(ctx, conversation.FamilyKey, conversation.BotKey)
	if err != nil {
		return err
	}
	text, mine := renderForLearning(rows, item.Kind == bot.KindPersona)
	last := rows[len(rows)-1]
	if mine >= learnMinRunes {
		occurred := last.CreatedAt
		header := fmt.Sprintf(learnHeader, rows[0].CreatedAt.Format("2006-01-02 15:04")+" ~ "+last.CreatedAt.Format("15:04"))
		// 对话里学到的东西归说话的人，默认只有他自己看得到
		owned := viewer.With(ctx, viewer.Viewer{Account: conversation.AccountKey})
		if _, err := notes.CreateNote(owned, conversation.FamilyKey, intake.NoteInput{
			Channel: learnerChannel, Text: header + text, OccurredAt: &occurred, CreatedBy: "chat:" + conversation.ConversationKey,
		}); err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", conversation.ID).
		Update("extracted_message_id", last.ID).Error
}

// renderForLearning 把消息转成给抽取模型读的文本，返回文本和「我说的话」的字数。
// 分身对话里分身的回答整个不给（模拟出来的话最容易被误当成事实）。
// 太长时保留最新的部分。
func renderForLearning(rows []messageRow, persona bool) (string, int) {
	var lines []string
	mine := 0
	for _, row := range rows {
		content := strings.TrimSpace(row.Content)
		if content == "" {
			continue
		}
		switch row.Role {
		case llm.RoleUser:
			if hasAttachment(row) {
				// 带照片的话已经和照片一起整理过了，这里只留个影子帮助理解上下文
				lines = append(lines, "【我】（拍了照片并说明，已单独整理）")
				continue
			}
			mine += len([]rune(content))
			lines = append(lines, "【我】"+content)
		case llm.RoleAssistant:
			if !persona {
				lines = append(lines, "【助理】"+oneLine(content, 200))
			}
		}
	}
	size := 0
	start := len(lines)
	for start > 0 && size+len([]rune(lines[start-1])) <= learnMaxRunes {
		start--
		size += len([]rune(lines[start]))
	}
	return strings.Join(lines[start:], "\n"), mine
}
