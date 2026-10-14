package reminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"common/keys"

	webpush "github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"
)

type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type PushDelivery struct {
	DeliveryKey     string    `json:"deliveryKey"`
	ReminderKey     string    `json:"reminderKey"`
	SubscriptionKey string    `json:"subscriptionKey"`
	Status          string    `json:"status"`
	HTTPStatus      int       `json:"httpStatus"`
	Error           string    `json:"error"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (s *Service) Subscribe(ctx context.Context, familyKey, accountKey, userAgent string, input PushSubscription) error {
	if !s.PushEnabled() {
		return errors.New("服务端还没配置 Web Push")
	}
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	if input.Endpoint == "" || input.Keys.P256dh == "" || input.Keys.Auth == "" {
		return errors.New("推送订阅不完整")
	}
	var row subscriptionRow
	hash := endpointHash(input.Endpoint)
	err := s.db.WithContext(ctx).Where("family_key = ? AND endpoint_hash = ?", familyKey, hash).First(&row).Error
	now := time.Now()
	if err == nil {
		return s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"account_key": accountKey, "endpoint": input.Endpoint,
			"p256dh": input.Keys.P256dh, "auth": input.Keys.Auth, "user_agent": userAgent, "enabled": 1, "last_error": "", "updated_at": now}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	row = subscriptionRow{SubscriptionKey: keys.New("push"), FamilyKey: familyKey, AccountKey: accountKey, EndpointHash: hash,
		Endpoint: input.Endpoint, P256dh: input.Keys.P256dh, Auth: input.Keys.Auth, UserAgent: userAgent,
		Enabled: 1, CreatedAt: now, UpdatedAt: now}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *Service) Unsubscribe(ctx context.Context, familyKey, accountKey, endpoint string) error {
	return s.db.WithContext(ctx).Model(&subscriptionRow{}).
		Where("family_key = ? AND account_key = ? AND endpoint_hash = ?", familyKey, accountKey, endpointHash(endpoint)).
		Updates(map[string]any{"enabled": 0, "updated_at": time.Now()}).Error
}

func (s *Service) ListDeliveries(ctx context.Context, familyKey string) ([]PushDelivery, error) {
	var rows []deliveryRow
	if err := s.db.WithContext(ctx).Where("family_key = ?", familyKey).Order("id DESC").Limit(300).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PushDelivery, 0, len(rows))
	for _, row := range rows {
		result = append(result, PushDelivery{DeliveryKey: row.DeliveryKey, ReminderKey: row.ReminderKey,
			SubscriptionKey: row.SubscriptionKey, Status: row.Status, HTTPStatus: row.HTTPStatus, Error: row.Error, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func (s *Service) TestPush(ctx context.Context, familyKey string) error {
	if !s.PushEnabled() {
		return errors.New("服务端还没配置 Web Push")
	}
	now := time.Now()
	item := Reminder{ReminderKey: keys.New("test"), Kind: "test", Title: "家庭助理推送已开启",
		Body: "以后生日、承诺和回老家简报会在这里提醒你。", TargetURL: "/today", RemindAt: now}
	sent, _, err := s.deliver(ctx, familyKey, item)
	if err != nil {
		return err
	}
	if sent == 0 {
		return errors.New("没有可用的推送订阅")
	}
	return nil
}

func (s *Service) deliver(ctx context.Context, familyKey string, item Reminder) (int, int, error) {
	var subscriptions []subscriptionRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND enabled = 1", familyKey).Find(&subscriptions).Error; err != nil {
		return 0, 0, err
	}
	if len(subscriptions) == 0 {
		return 0, 0, nil
	}
	// Web Push 单条载荷有 4KB 上限；回老家简报含家庭原话和雷区，不把这些内容显示在锁屏上。
	notificationBody := oneLine(item.Body, 500)
	if item.Kind == "trip" {
		notificationBody = "出发前简报已准备好，打开查看称呼、雷区和当地规矩。"
	}
	payload, _ := json.Marshal(map[string]any{"title": item.Title, "body": notificationBody, "url": defaultString(item.TargetURL, "/today"), "tag": item.ReminderKey})
	sent, failed := 0, 0
	var lastErr error
	for _, subscription := range subscriptions {
		response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{Endpoint: subscription.Endpoint,
			Keys: webpush.Keys{P256dh: subscription.P256dh, Auth: subscription.Auth}}, &webpush.Options{
			Subscriber: s.config.VAPIDSubject, VAPIDPublicKey: s.config.VAPIDPublicKey, VAPIDPrivateKey: s.config.VAPIDPrivateKey,
			TTL: 24 * 3600, Urgency: webpush.UrgencyNormal, Topic: pushTopic(item.ReminderKey),
		})
		status, errorText := 0, ""
		if response != nil {
			status = response.StatusCode
			body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
			response.Body.Close()
			if status < 200 || status >= 300 {
				errorText = strings.TrimSpace(string(body))
				err = fmt.Errorf("push HTTP %d: %s", status, errorText)
			}
		}
		if err == nil {
			sent++
			s.db.WithContext(ctx).Model(&subscription).Updates(map[string]any{"last_success_at": time.Now(), "last_error": "", "updated_at": time.Now()})
		} else {
			failed++
			lastErr = err
			errorText = truncate(err.Error(), 1000)
			updates := map[string]any{"last_error": errorText, "updated_at": time.Now()}
			if status == http.StatusGone || status == http.StatusNotFound {
				updates["enabled"] = 0
			}
			s.db.WithContext(ctx).Model(&subscription).Updates(updates)
		}
		s.db.WithContext(ctx).Create(&deliveryRow{DeliveryKey: keys.New("delivery"), FamilyKey: familyKey, ReminderKey: item.ReminderKey,
			SubscriptionKey: subscription.SubscriptionKey, Status: map[bool]string{true: "sent", false: "failed"}[err == nil],
			HTTPStatus: status, Error: errorText, CreatedAt: time.Now()})
	}
	return sent, failed, lastErr
}

func pushTopic(key string) string {
	value := strings.ReplaceAll(key, "_", "-")
	if len(value) > 32 {
		value = value[len(value)-32:]
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncate(value string, size int) string {
	runes := []rune(value)
	if len(runes) > size {
		return string(runes[:size])
	}
	return value
}
