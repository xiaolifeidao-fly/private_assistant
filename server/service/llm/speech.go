package llm

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// TaskASR 是语音转文字的路由。提供商只用它的地址和 Key：
//   - MiniMax（地址里带 minimax）：POST {scheme}://{host}/v1/speech_to_text，模型 asr-1.0；
//     不管提供商地址写的是 /anthropic 还是 /v1，都换成这个接口，和对话共用一个 Key。
//   - 其它：按 OpenAI 兼容协议 POST {base_url}/audio/transcriptions。
//
// 两种接口都是 multipart（model + file），返回 {"text": ...}。
const TaskASR = "asr"

const asrTimeout = 2 * time.Minute

var ErrNoASR = errors.New("没有配置语音识别（模型路由 asr）")

// MaxAudioBytes：识别服务上限 50MB / 500 秒；浏览器录的 m4a 一分钟不到 1MB，20MB 足够。
const MaxAudioBytes = 20 << 20

// 识别服务按扩展名解码，只放行它认的格式（不认 webm）。
var audioExtensions = map[string]bool{".m4a": true, ".mp4": true, ".mp3": true, ".wav": true, ".ogg": true, ".opus": true, ".aac": true, ".flac": true}

// TranscribeUpload 是两个 API 的「上传一段录音转文字」：校验格式和大小后调 Transcribe。录音不保存。
func (g *Gateway) TranscribeUpload(ctx context.Context, header *multipart.FileHeader) (string, error) {
	extension := strings.ToLower(filepath.Ext(header.Filename))
	if !audioExtensions[extension] {
		return "", fmt.Errorf("这个录音格式不支持（%s）", extension)
	}
	if header.Size > MaxAudioBytes {
		return "", errors.New("录音太长了，分几段说吧")
	}
	file, err := header.Open()
	if err != nil {
		return "", errors.New("读取录音失败")
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, MaxAudioBytes+1))
	if err != nil || len(audio) > MaxAudioBytes {
		return "", errors.New("录音太长了，分几段说吧")
	}
	text, err := g.Transcribe(ctx, audio, "voice"+extension)
	if errors.Is(err, ErrNoASR) {
		return "", errors.New("还没配置语音识别：到管理端「模型」里配置 asr 路由")
	}
	return text, err
}

// Transcribe 把一段录音转成文字。filename 的扩展名决定服务端按什么格式解码（wav、m4a、mp3……）。
func (g *Gateway) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	route, err := g.Resolve(ctx, TaskASR)
	if err != nil {
		return "", ErrNoASR
	}
	endpoint, err := asrEndpoint(route.BaseURL)
	if err != nil {
		return "", err
	}
	started := time.Now()
	text, err := postTranscription(ctx, route, endpoint, audio, filename)
	g.logCall(route, "", Response{StopReason: StopEnd}, err, time.Since(started))
	return text, err
}

func asrEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("语音识别的提供商没有填地址")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("语音识别的提供商地址不对: %s", baseURL)
	}
	if isMiniMax(baseURL) {
		return parsed.Scheme + "://" + parsed.Host + "/v1/speech_to_text", nil
	}
	return baseURL + "/audio/transcriptions", nil
}

func postTranscription(ctx context.Context, route Route, endpoint string, audio []byte, filename string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("model", route.Model)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, asrTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+route.APIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("语音识别请求失败: %w", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		return "", &ProviderError{Status: response.StatusCode, Message: fmt.Sprintf("语音识别失败 %d: %s", response.StatusCode, truncate(string(data), 500))}
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("语音识别结果解析失败: %s", truncate(string(data), 200))
	}
	return strings.TrimSpace(result.Text), nil
}

// silentWAV 生成一秒 16kHz 单声道静音，测试路由用。
func silentWAV() []byte {
	const rate, samples = 16000, 16000
	var buffer bytes.Buffer
	write := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(&buffer, binary.LittleEndian, value)
		}
	}
	buffer.WriteString("RIFF")
	write(uint32(36 + samples*2))
	buffer.WriteString("WAVEfmt ")
	write(uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate*2), uint16(2), uint16(16))
	buffer.WriteString("data")
	write(uint32(samples * 2))
	buffer.Write(make([]byte, samples*2))
	return buffer.Bytes()
}
