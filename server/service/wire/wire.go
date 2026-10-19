// Package wire 把各领域服务装配到一起，两个 API 进程共用同一套装配。
package wire

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"common/viewer"
	"service/bot"
	"service/chat"
	"service/diet"
	"service/family"
	"service/identity"
	"service/insight"
	"service/intake"
	"service/llm"
	"service/memory"
	"service/persona"
	"service/reminder"
	"service/setting"
	"service/skill"

	"common/objectstore"

	"gorm.io/gorm"
)

const DefaultFamily = "default"

type Services struct {
	Identity *identity.Service
	Family   *family.Service
	Memory   *memory.Service
	Bots     *bot.Service
	LLM      *llm.Gateway
	Chat     *chat.Service
	Intake   *intake.Service
	Persona  *persona.Service
	Reminder *reminder.Service
	Skills   *skill.Service
	Settings *setting.Service
	Insight  *insight.Service
	Diet     *diet.Service
}

type Options struct {
	Identity identity.Config
	// 配置文件里的初始值：只在对应数据不存在时写入，不覆盖管理端改过的配置。
	AnthropicAPIKey  string
	AnthropicBaseURL string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	// VectorStore：file（默认，向量存服务器本地文件）/ mysql（存 pa_memory_embedding）。
	// VectorDir 默认 {StorageDir}/vectors。两个 API 进程必须指向同一个目录。
	VectorStore string
	VectorDir   string
	// StorageDir 存截图等附件的目录。两个 API 进程必须指向同一个目录。
	// 配了 OSS 以后新附件写 OSS，这个目录只用来读切换前的旧附件。
	StorageDir string
	OSS        objectstore.OSSConfig
	// AutoConfirm 自动抽取的记忆置信度不低于它才直接生效，否则进待确认。
	AutoConfirm     float64
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

// LoadStorage 从配置读附件存储：storage.dir 和 storage.oss.*（bucket 留空表示不用 OSS），以及向量存哪：vector.*。
func (o *Options) LoadStorage(property func(string) string) {
	o.StorageDir = property("storage.dir")
	o.VectorStore = property("vector.store")
	o.VectorDir = property("vector.dir")
	o.OSS = objectstore.OSSConfig{
		Region: property("storage.oss.region"), Endpoint: property("storage.oss.endpoint"), Bucket: property("storage.oss.bucket"),
		AccessKeyID: property("storage.oss.access_key_id"), AccessKeySecret: property("storage.oss.access_key_secret"),
		Prefix: property("storage.oss.prefix"),
	}
}

func vectorStore(database *gorm.DB, options Options) memory.VectorStore {
	if options.VectorStore == "mysql" {
		log.Printf("向量存储：MySQL（pa_memory_embedding）")
		return memory.MySQLVectorStore{DB: database}
	}
	dir := options.VectorDir
	if dir == "" {
		dir = filepath.Join(options.StorageDir, "vectors")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("向量目录 %s 不可用: %v", dir, err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		log.Fatalf("向量目录 %s 不可用: %v", absolute, err)
	}
	log.Printf("向量存储：本地文件 %s", absolute)
	return memory.FileVectorStore{Dir: absolute}
}

func Build(database *gorm.DB, options Options) *Services {
	// 记忆、原始记录、提醒的可见范围在数据访问层强制（common/viewer）
	if err := database.Use(viewer.Plugin{}); err != nil && !errors.Is(err, gorm.ErrRegistered) {
		log.Fatalf("注册可见范围插件失败: %v", err)
	}
	if options.StorageDir == "" {
		options.StorageDir = "../data"
	}
	local, err := objectstore.NewLocal(options.StorageDir)
	if err != nil {
		log.Fatalf("附件目录 %s 不可用: %v", options.StorageDir, err)
	}
	var storage objectstore.Store = local
	if options.OSS.Bucket != "" {
		bucket, err := objectstore.NewOSS(options.OSS)
		if err != nil {
			log.Fatalf("OSS 不可用: %v", err)
		}
		storage = objectstore.Fallback{Primary: bucket, Legacy: local}
		log.Printf("附件存储：OSS %s/%s", options.OSS.Bucket, options.OSS.Prefix)
	}
	gateway := llm.New(database)
	familyService := family.New(database)
	memoryService := memory.New(database, gateway, vectorStore(database, options), func(ctx context.Context, familyKey string) map[string]string {
		labels := map[string]string{}
		persons, _ := familyService.ListPersons(ctx, familyKey)
		for _, person := range persons {
			labels[person.PersonKey] = person.RoleLabel
			if person.Name != "" && person.Name != person.RoleLabel {
				labels[person.PersonKey] += "（" + person.Name + "）"
			}
		}
		return labels
	})
	botService := bot.New(database)
	skillService := skill.New(database, familyService, memoryService, gateway)
	settingService := setting.New(database)
	personaService := persona.New(database, familyService, memoryService, gateway, settingService)
	reminderService := reminder.New(database, familyService, memoryService, reminder.Config{
		VAPIDPublicKey: options.VAPIDPublicKey, VAPIDPrivateKey: options.VAPIDPrivateKey, VAPIDSubject: options.VAPIDSubject,
	})
	dietService := diet.New(database, familyService, gateway, storage)
	intakeService := intake.New(database, familyService, memoryService, gateway, storage, intake.Config{AutoConfirm: options.AutoConfirm})
	identityService := identity.New(database, options.Identity)
	// 后台任务没有登录账号，按「我自己」的账号看记录：看得到我的私有记录，看不到奶奶的
	viewer.SystemAccounts = identityService.SystemAccounts
	dietService.SetNotifier(reminderService, identityService.PlanAccounts)
	return &Services{
		Identity: identityService,
		Family:   familyService,
		Memory:   memoryService,
		Bots:     botService,
		LLM:      gateway,
		Chat:     chat.New(database, familyService, memoryService, botService, gateway, personaService, reminderService, skillService, settingService, intakeService, dietService),
		Persona:  personaService,
		Reminder: reminderService,
		Skills:   skillService,
		Settings: settingService,
		Insight:  insight.New(database, memoryService, familyService, gateway, reminderService, settingService),
		Intake:   intakeService,
		Diet:     dietService,
	}
}

// StartWorkers 启动后台任务：原始记录抽取、人设卡刷新、提醒、对话自动学习。两个进程都跑也没问题（按行抢占 / 分布式锁）。
func (s *Services) StartWorkers(ctx context.Context) {
	// 每个后台任务都要声明身份（common/viewer），没声明的查询会被拒绝：
	// 领取待整理的记录、到点推送要看到所有人的（Unscoped）；人设卡、周洞察、对话学习按我自己的账号看（AsSystem）
	go s.Intake.Run(viewer.Unscoped(ctx))
	go s.Persona.RunScheduler(viewer.AsSystem(ctx))
	go s.Reminder.RunScheduler(viewer.Unscoped(ctx))
	go s.Chat.RunLearner(viewer.AsSystem(ctx), s.Intake)
	go s.Insight.RunScheduler(viewer.AsSystem(ctx))
	// 饮食周点评：饮食表全家共用；推送时按每个收件人的账号建提醒
	go s.Diet.RunScheduler(viewer.AsSystem(ctx))
}

// Seed 写入默认机器人和配置文件里给的模型提供商，并在后台补算缺失的向量。
func (s *Services) Seed(options Options) {
	ctx, cancel := context.WithTimeout(viewer.AsSystem(context.Background()), 30*time.Second)
	defer cancel()
	if err := s.Bots.EnsureDefaults(ctx, DefaultFamily); err != nil {
		log.Printf("写入默认机器人失败: %v", err)
	}
	if err := s.Skills.EnsureDefaults(ctx, DefaultFamily); err != nil {
		log.Printf("写入默认手册失败: %v", err)
	}
	if options.AnthropicAPIKey != "" {
		if err := s.LLM.EnsureProvider(ctx, llm.Provider{ProviderKey: "anthropic", Name: "Anthropic Claude", Kind: llm.KindAnthropic,
			BaseURL: options.AnthropicBaseURL, APIKey: options.AnthropicAPIKey}); err != nil {
			log.Printf("写入 Anthropic 提供商失败: %v", err)
		}
	}
	if options.EmbeddingBaseURL != "" {
		if err := s.LLM.EnsureProvider(ctx, llm.Provider{ProviderKey: "embedding", Name: "Embedding", Kind: llm.KindOpenAI,
			BaseURL: options.EmbeddingBaseURL, APIKey: options.EmbeddingAPIKey}); err != nil {
			log.Printf("写入 Embedding 提供商失败: %v", err)
		}
	}
	s.ReindexInBackground()
}

// ReindexInBackground 在后台给当前向量模型下还没有向量的记忆补算：启动时跑一次，
// 管理端换了 embedding 路由后再跑一次（换模型 = 全部重算，旧模型的向量被覆盖）。
func (s *Services) ReindexInBackground() {
	go func() {
		// 补算向量不看是谁的，每个人的记录都要能被自己搜到
		ctx, cancel := context.WithTimeout(viewer.Unscoped(context.Background()), 30*time.Minute)
		defer cancel()
		if err := s.Memory.PrepareVectors(ctx, DefaultFamily); err != nil {
			log.Printf("%v", err)
		}
		if count, err := s.Memory.ReindexMissing(ctx, DefaultFamily); err != nil {
			log.Printf("补算记忆向量失败: %v", err)
		} else if count > 0 {
			log.Printf("补算了 %d 条记忆向量", count)
		}
		if count, err := s.Memory.ReindexPassages(ctx, DefaultFamily); err != nil {
			log.Printf("补算原文片段向量失败: %v", err)
		} else if count > 0 {
			log.Printf("补算了 %d 段原文向量", count)
		}
	}()
}
