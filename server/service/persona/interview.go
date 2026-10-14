package persona

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"service/family"
	"service/llm"
	"service/memory"
)

// 对话补充人设卡：我和模型聊，模型挑卡里信息不够的地方问我；聊够了再把对话整理成草稿，
// 我看过、改过之后存为新版本。对话本身不落库，由前端每轮带上完整记录。
const (
	taskInterview = "persona.interview"

	maxInterviewTurns = 60
	openingPrompt     = "我们开始吧。先看看现在的人设卡和记忆里哪些地方信息不够，再问我。"
)

// Turn 是对话补充里的一条消息。
type Turn struct {
	Role string `json:"role"` // user / assistant
	Text string `json:"text"`
}

// Draft 是根据对话整理出的人设卡草稿，还没存。
type Draft struct {
	Content string `json:"content"`
	Changes string `json:"changes"`
}

// material 取出某人的档案、记忆、当前人设卡，拼成给模型看的参考材料。
func (s *Service) material(ctx context.Context, familyKey, personKey string) (family.Person, string, error) {
	person, err := s.family.GetPerson(ctx, familyKey, personKey)
	if err != nil {
		return family.Person{}, "", err
	}
	active, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Person: personKey, Statuses: []string{memory.StatusActive}, Size: 200})
	if err != nil {
		return family.Person{}, "", err
	}
	old, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Person: personKey, Statuses: []string{memory.StatusSuperseded}, Size: 40})
	if err != nil {
		return family.Person{}, "", err
	}
	var input strings.Builder
	input.WriteString(s.background(ctx, familyKey, person, active, old))
	if len(active) == 0 {
		input.WriteString("（还没有关于她的记忆）\n")
	}
	current, hasCurrent, err := s.Active(ctx, familyKey, personKey)
	if err != nil {
		return family.Person{}, "", err
	}
	if hasCurrent {
		fmt.Fprintf(&input, "\n## 当前人设卡（v%d，%s）\n%s\n", current.Version, current.CreatedAt.Format("2006-01-02"), current.Content)
	} else {
		input.WriteString("\n## 当前人设卡\n还没有。\n")
	}
	return person, input.String(), nil
}

func normalizeTurns(turns []Turn) ([]Turn, error) {
	if len(turns) > maxInterviewTurns {
		return nil, fmt.Errorf("对话太长了（超过 %d 条），先生成草稿保存一版，再开新的对话", maxInterviewTurns)
	}
	result := make([]Turn, 0, len(turns))
	for _, turn := range turns {
		text := strings.TrimSpace(turn.Text)
		if text == "" {
			continue
		}
		if turn.Role != llm.RoleUser && turn.Role != llm.RoleAssistant {
			return nil, fmt.Errorf("未知的消息角色 %s", turn.Role)
		}
		// 连续同一角色的消息合并，保证一问一答交替
		if n := len(result); n > 0 && result[n-1].Role == turn.Role {
			result[n-1].Text += "\n\n" + text
			continue
		}
		result = append(result, Turn{Role: turn.Role, Text: text})
	}
	return result, nil
}

// Interview 接着对话往下问。turns 是到目前为止的完整对话（最后一条应是我说的话；为空时由模型开场）。
// 回复以流式增量通过 onDelta 回调，返回完整回复文本。
func (s *Service) Interview(ctx context.Context, familyKey, personKey string, turns []Turn, onDelta func(string)) (string, error) {
	turns, err := normalizeTurns(turns)
	if err != nil {
		return "", err
	}
	if len(turns) > 0 && turns[len(turns)-1].Role != llm.RoleUser {
		return "", errors.New("最后一条应该是你说的话")
	}
	person, material, err := s.material(ctx, familyKey, personKey)
	if err != nil {
		return "", err
	}
	messages := []llm.Message{textMessage(llm.RoleUser, openingPrompt)}
	if len(turns) > 0 && turns[0].Role == llm.RoleUser {
		// 我先开的口：开场白和第一句话合成一条
		messages[0] = textMessage(llm.RoleUser, openingPrompt+"\n\n"+turns[0].Text)
		turns = turns[1:]
	}
	for _, turn := range turns {
		messages = append(messages, textMessage(turn.Role, turn.Text))
	}

	response, err := s.llm.Chat(ctx, llm.Request{
		Task: taskInterview,
		System: []llm.SystemPart{
			{Text: interviewPrompt(person)},
			{Text: "# 参考材料\n\n" + material, Cache: true},
		},
		Messages: messages,
		RefKey:   personKey,
	}, func(delta llm.Delta) {
		if delta.Type == "text" && onDelta != nil {
			onDelta(delta.Text)
		}
	})
	if err != nil {
		return "", err
	}
	if response.StopReason == llm.StopRefusal {
		return "", errors.New("模型拒绝回答这一轮，换个说法再试")
	}
	return response.Message.Text(), nil
}

// DraftFromInterview 把对话里我补充的信息和记忆、当前人设卡合起来，整理成一版完整的人设卡草稿。
func (s *Service) DraftFromInterview(ctx context.Context, familyKey, personKey string, turns []Turn) (Draft, error) {
	turns, err := normalizeTurns(turns)
	if err != nil {
		return Draft{}, err
	}
	hasUser := false
	for _, turn := range turns {
		hasUser = hasUser || turn.Role == llm.RoleUser
	}
	if !hasUser {
		return Draft{}, errors.New("还没聊什么，先回答几个问题再生成草稿")
	}
	person, material, err := s.material(ctx, familyKey, personKey)
	if err != nil {
		return Draft{}, err
	}
	var input strings.Builder
	input.WriteString(material)
	input.WriteString("\n## 我刚才在对话里补充的信息\n")
	for _, turn := range turns {
		if turn.Role == llm.RoleUser {
			input.WriteString("我：" + turn.Text + "\n\n")
		} else {
			input.WriteString("你：" + turn.Text + "\n\n")
		}
	}

	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskCard,
		System:     []llm.SystemPart{{Text: systemPrompt(person) + draftPromptSuffix, Cache: true}},
		Messages:   []llm.Message{textMessage(llm.RoleUser, input.String())},
		JSONSchema: cardSchema(),
		RefKey:     personKey,
	}, nil)
	if err != nil {
		return Draft{}, err
	}
	if response.StopReason == llm.StopRefusal || response.StopReason == llm.StopMaxToken {
		return Draft{}, fmt.Errorf("草稿生成没有完成（%s）", response.StopReason)
	}
	var output cardOutput
	if err := json.Unmarshal([]byte(extractJSON(response.Message.Text())), &output); err != nil {
		return Draft{}, fmt.Errorf("解析草稿失败: %w", err)
	}
	if strings.TrimSpace(output.Content) == "" {
		return Draft{}, errors.New("模型返回了空的草稿")
	}
	return Draft{Content: strings.TrimSpace(output.Content), Changes: strings.TrimSpace(output.Changes)}, nil
}

// SaveInterview 把（可能改过的）草稿存为新版本并生效。草稿参考了当时全部的记忆，所以记忆截止时间取最新。
func (s *Service) SaveInterview(ctx context.Context, familyKey, personKey, content, changes string) (Card, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Card{}, errors.New("人设卡内容不能为空")
	}
	if _, err := s.family.GetPerson(ctx, familyKey, personKey); err != nil {
		return Card{}, err
	}
	count, latest, err := s.memory.LatestChange(ctx, familyKey, personKey)
	if err != nil {
		return Card{}, err
	}
	return s.saveVersion(ctx, familyKey, personKey, content, defaultString(changes, "对话补充"), int(count), latest, "chat")
}

func textMessage(role, text string) llm.Message {
	return llm.Message{Role: role, Blocks: []llm.Block{{Type: llm.BlockText, Text: text}}}
}

const draftPromptSuffix = `

这次除了记忆，还有我刚才和你的一段对话。对话里我说的内容是第一手信息，可以直接写进卡里；
和较早的记忆冲突时以对话为准，并在相应小节点明变化；和近期记忆冲突时两种说法都写上。
你在对话里的提问和猜测不算信息，只采用我确认过的内容。
changes 里用一两句话写这次通过对话补充或修正了什么。`

func interviewPrompt(person family.Person) string {
	return `你在帮我（这个家的丈夫/爸爸）通过聊天补全「` + person.RoleLabel + `」的人设卡。人设卡让 AI 分身模拟她的视角和说话方式，也让家庭助理在给我出主意时理解她。
参考材料里有她的档案、关系、记忆和当前的人设卡。人设卡按下面的小节写：
` + cardSections(person) + `

怎么聊：
- 先看哪些小节是空的、写着「暂无足够信息」或者很笼统，从最影响理解她的地方问起；记忆里已经清楚的不要再问。
- 每次只问一到两个具体问题，尽量问场景和原话，例如「上次她不高兴，当时发生了什么、她说了什么」，而不是「她是什么性格」。
- 我回答之后，先用一句话复述你记下的要点（方便我纠正），再问下一个。我说的和记忆或当前人设卡对不上时，直接指出来问哪个是现在的情况。
- 不评判对错，不站队，不给建议；这里只是把她了解清楚。
- 不要输出整张人设卡。聊到你觉得主要小节都有内容了，告诉我可以点「生成草稿」。
- 回答简短，用口语，不用标题和长列表。`
}
