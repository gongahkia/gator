package channel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/store"
)

type Invoker interface {
	InvokeAgent(context.Context, string, runtime.AgentInvocation) (runtime.AgentResponse, error)
}

type Service struct {
	store     *store.Store
	invoke    Invoker
	artifacts string
	http      *http.Client
	mu        sync.Mutex
	gateways  map[string]context.CancelFunc
}

func New(st *store.Store, invoke Invoker, artifactsDir string) *Service {
	return &Service{store: st, invoke: invoke, artifacts: filepath.Join(artifactsDir, "channels"), http: &http.Client{Timeout: 30 * time.Second}, gateways: map[string]context.CancelFunc{}}
}

func (s *Service) CreateAccount(ctx context.Context, value domain.ChannelAccount) (domain.ChannelAccount, error) {
	if value.Adapter != "telegram" && value.Adapter != "slack" && value.Adapter != "discord" && value.Adapter != "whatsapp" {
		return domain.ChannelAccount{}, fmt.Errorf("unsupported channel adapter")
	}
	if value.ID == "" {
		value.ID = randomID()
	}
	if !value.Enabled {
		value.Enabled = true
	}
	if _, err := s.store.GetRun(ctx, value.RunID); err != nil {
		return domain.ChannelAccount{}, err
	}
	if value.Adapter == "discord" {
		if _, ok := stringSetting(value.Settings, "public_key"); !ok {
			return domain.ChannelAccount{}, fmt.Errorf("discord settings require public_key")
		}
	}
	if value.Adapter == "whatsapp" {
		if _, ok := stringSetting(value.Settings, "phone_number_id"); !ok {
			return domain.ChannelAccount{}, fmt.Errorf("whatsapp settings require phone_number_id")
		}
	}
	for name, ref := range value.SecretRefs {
		if name == "" || !validEnvRef(ref) {
			return domain.ChannelAccount{}, fmt.Errorf("invalid secret reference for %q", name)
		}
	}
	return s.store.CreateChannelAccount(ctx, value)
}

func (s *Service) Pair(ctx context.Context, accountID, externalID string, expiresAt *time.Time) (domain.ChannelPairing, error) {
	return s.store.PairChannelIdentity(ctx, domain.ChannelPairing{AccountID: accountID, ExternalID: externalID, ExpiresAt: expiresAt})
}
func (s *Service) Unpair(ctx context.Context, accountID, externalID string) error {
	return s.store.UnpairChannelIdentity(ctx, accountID, externalID)
}
func (s *Service) Accounts(ctx context.Context) ([]domain.ChannelAccount, error) {
	return s.store.ChannelAccounts(ctx)
}
func (s *Service) ResetSession(ctx context.Context, accountID, externalID string) error {
	return s.store.ResetSession(ctx, accountID, externalID)
}
func (s *Service) ExportSession(ctx context.Context, accountID, externalID string) ([]domain.ChannelMessage, error) {
	return s.store.ChannelMessages(ctx, accountID, externalID)
}

func (s *Service) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	account, err := s.store.ChannelAccount(r.Context(), r.PathValue("account"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !account.Enabled {
		writeError(w, http.StatusGone, fmt.Errorf("channel account is disabled"))
		return
	}
	switch account.Adapter {
	case "telegram":
		s.telegram(w, r, account)
	case "slack":
		s.slack(w, r, account)
	case "discord":
		s.discord(w, r, account)
	case "whatsapp":
		s.whatsapp(w, r, account)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported adapter"))
	}
}

func (s *Service) telegram(w http.ResponseWriter, r *http.Request, account domain.ChannelAccount) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	secret := s.secret(account, "webhook_secret")
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))) != 1 {
		writeError(w, 401, fmt.Errorf("invalid telegram webhook secret"))
		return
	}
	var update struct {
		UpdateID int64 `json:"update_id"`
		Message  struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			From struct {
				ID int64 `json:"id"`
			} `json:"from"`
			Text     string `json:"text"`
			Document *struct {
				FileID   string `json:"file_id"`
				FileName string `json:"file_name"`
			} `json:"document"`
		} `json:"message"`
	}
	if json.Unmarshal(body, &update) != nil || update.Message.From.ID == 0 {
		writeError(w, 400, fmt.Errorf("unsupported telegram update"))
		return
	}
	attachments := []map[string]any{}
	if update.Message.Document != nil {
		attachments = append(attachments, map[string]any{"platform_file_id": update.Message.Document.FileID, "name": update.Message.Document.FileName, "adapter": "telegram"})
	}
	s.accept(w, account, inbound{PlatformID: strconv.FormatInt(update.UpdateID, 10), ExternalID: strconv.FormatInt(update.Message.From.ID, 10), ReplyID: strconv.FormatInt(update.Message.Chat.ID, 10), Text: update.Message.Text, Attachments: attachments})
}

func (s *Service) slack(w http.ResponseWriter, r *http.Request, account domain.ChannelAccount) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	secret := s.secret(account, "signing_secret")
	if !validSlack(secret, r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body) {
		writeError(w, 401, fmt.Errorf("invalid slack signature"))
		return
	}
	var event struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		EventID   string `json:"event_id"`
		Event     struct {
			Type    string           `json:"type"`
			User    string           `json:"user"`
			Text    string           `json:"text"`
			Channel string           `json:"channel"`
			BotID   string           `json:"bot_id"`
			Files   []map[string]any `json:"files"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, 400, err)
		return
	}
	if event.Type == "url_verification" {
		writeJSON(w, 200, map[string]string{"challenge": event.Challenge})
		return
	}
	if event.Type != "event_callback" || event.Event.Type != "message" || event.Event.User == "" || event.Event.BotID != "" {
		writeJSON(w, 200, map[string]string{"status": "ignored"})
		return
	}
	s.accept(w, account, inbound{PlatformID: event.EventID, ExternalID: event.Event.User, ReplyID: event.Event.Channel, Text: event.Event.Text, Attachments: event.Event.Files})
}

func (s *Service) discord(w http.ResponseWriter, r *http.Request, account domain.ChannelAccount) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	public, ok := stringSetting(account.Settings, "public_key")
	if !ok || !validDiscord(public, r.Header.Get("X-Signature-Ed25519"), r.Header.Get("X-Signature-Timestamp"), body) {
		writeError(w, 401, fmt.Errorf("invalid discord signature"))
		return
	}
	var interaction struct {
		ID            string `json:"id"`
		Type          int    `json:"type"`
		Token         string `json:"token"`
		ApplicationID string `json:"application_id"`
		ChannelID     string `json:"channel_id"`
		Member        struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"member"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Data struct {
			Name    string `json:"name"`
			Options []struct {
				Value any `json:"value"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &interaction); err != nil {
		writeError(w, 400, err)
		return
	}
	if interaction.Type == 1 {
		writeJSON(w, 200, map[string]int{"type": 1})
		return
	}
	external := interaction.User.ID
	if external == "" {
		external = interaction.Member.User.ID
	}
	if external == "" {
		writeError(w, 400, fmt.Errorf("discord user missing"))
		return
	}
	text := interaction.Data.Name
	for _, option := range interaction.Data.Options {
		text += " " + fmt.Sprint(option.Value)
	}
	writeJSON(w, 200, map[string]int{"type": 5})
	s.acceptAsync(account, inbound{PlatformID: interaction.ID, ExternalID: external, ReplyID: interaction.ChannelID, Text: strings.TrimSpace(text), Attachments: []map[string]any{{"interaction_token": interaction.Token, "application_id": interaction.ApplicationID, "channel_id": interaction.ChannelID}}})
}

func (s *Service) whatsapp(w http.ResponseWriter, r *http.Request, account domain.ChannelAccount) {
	if r.Method == http.MethodGet {
		token := s.secret(account, "verify_token")
		if token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(r.URL.Query().Get("hub.verify_token"))) == 1 {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(r.URL.Query().Get("hub.challenge")))
			return
		}
		writeError(w, 401, fmt.Errorf("invalid whatsapp verification"))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	secret := s.secret(account, "app_secret")
	if !validHMAC(secret, r.Header.Get("X-Hub-Signature-256"), body, "sha256=") {
		writeError(w, 401, fmt.Errorf("invalid whatsapp signature"))
		return
	}
	var update struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						ID   string `json:"id"`
						From string `json:"from"`
						Type string `json:"type"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
						Document map[string]any `json:"document"`
						Image    map[string]any `json:"image"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		writeError(w, 400, err)
		return
	}
	for _, entry := range update.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				attachments := []map[string]any{}
				if message.Document != nil {
					attachments = append(attachments, message.Document)
				}
				if message.Image != nil {
					attachments = append(attachments, message.Image)
				}
				s.acceptAsync(account, inbound{PlatformID: message.ID, ExternalID: message.From, ReplyID: message.From, Text: message.Text.Body, Attachments: attachments})
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}

type inbound struct {
	PlatformID, ExternalID, ReplyID, Text string
	Attachments                           []map[string]any
}

func (s *Service) accept(w http.ResponseWriter, account domain.ChannelAccount, value inbound) {
	writeJSON(w, 200, map[string]string{"status": "accepted"})
	s.acceptAsync(account, value)
}
func (s *Service) acceptAsync(account domain.ChannelAccount, value inbound) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_ = s.ingest(ctx, account, value)
	}()
}
func (s *Service) ingest(ctx context.Context, account domain.ChannelAccount, value inbound) error {
	paired, err := s.store.IsPaired(ctx, account.ID, value.ExternalID)
	if err != nil {
		return err
	}
	if !paired {
		_, _, _ = s.store.CreateChannelMessage(ctx, domain.ChannelMessage{AccountID: account.ID, ExternalID: value.ExternalID, Direction: "inbound", PlatformID: value.PlatformID, IdempotencyKey: "in:" + value.PlatformID, Text: value.Text, Attachments: value.Attachments, State: "rejected", Error: "identity is not paired"})
		return nil
	}
	message, created, err := s.store.CreateChannelMessage(ctx, domain.ChannelMessage{AccountID: account.ID, ExternalID: value.ExternalID, Direction: "inbound", PlatformID: value.PlatformID, IdempotencyKey: "in:" + value.PlatformID, Text: value.Text, Attachments: value.Attachments, State: "received"})
	if err != nil || !created {
		return err
	}
	session, err := s.store.Session(ctx, account.ID, value.ExternalID, randomID(), time.Now().UTC().Add(30*24*time.Hour))
	if err != nil {
		return err
	}
	response, err := s.invoke.InvokeAgent(ctx, account.RunID, runtime.AgentInvocation{SessionID: session.ID, ExternalID: value.ExternalID, Role: "operator", Prompt: value.Text, IdempotencyKey: "agent:" + account.ID + ":" + value.PlatformID, Attachments: value.Attachments})
	if err != nil {
		_, _, _ = s.store.CreateChannelMessage(ctx, domain.ChannelMessage{AccountID: account.ID, ExternalID: value.ExternalID, Direction: "outbound", IdempotencyKey: "out:" + value.PlatformID, Text: "Agent invocation failed.", Attachments: routeAttachment(value), State: "pending", Error: err.Error()})
		return err
	}
	if response.Final != "" {
		_ = s.store.UpdateSessionSummary(ctx, session.ID, trimSummary(session.Summary+"\nuser: "+value.Text+"\nassistant: "+response.Final))
		_, _, err = s.store.CreateChannelMessage(ctx, domain.ChannelMessage{AccountID: account.ID, ExternalID: value.ReplyID, Direction: "outbound", IdempotencyKey: "out:" + message.PlatformID, Text: response.Final, Attachments: routeAttachment(value), State: "pending"})
		return err
	}
	return nil
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.flush(ctx)
				_, _ = s.store.ExpireSessions(ctx)
			}
		}
	}()
	s.startDiscordGateways(ctx)
}
func (s *Service) flush(ctx context.Context) error {
	messages, err := s.store.PendingOutboundMessages(ctx, 50)
	if err != nil {
		return err
	}
	for _, message := range messages {
		account, err := s.store.ChannelAccount(ctx, message.AccountID)
		if err != nil {
			_ = s.store.CompleteChannelMessage(ctx, message.ID, "failed", err.Error())
			continue
		}
		if err = s.deliver(ctx, account, message); err != nil {
			_ = s.store.CompleteChannelMessage(ctx, message.ID, "failed", err.Error())
			continue
		}
		_ = s.store.CompleteChannelMessage(ctx, message.ID, "delivered", "")
	}
	return nil
}
func (s *Service) deliver(ctx context.Context, account domain.ChannelAccount, message domain.ChannelMessage) error {
	switch account.Adapter {
	case "telegram":
		return s.telegramSend(ctx, account, message)
	case "slack":
		return s.slackSend(ctx, account, message)
	case "discord":
		return s.discordSend(ctx, account, message)
	case "whatsapp":
		return s.whatsappSend(ctx, account, message)
	}
	return fmt.Errorf("unsupported adapter")
}

func (s *Service) telegramSend(ctx context.Context, account domain.ChannelAccount, message domain.ChannelMessage) error {
	token := s.secret(account, "bot_token")
	if token == "" {
		return fmt.Errorf("telegram bot_token secret ref is unset")
	}
	return s.postJSON(ctx, "https://api.telegram.org/bot"+token+"/sendMessage", nil, map[string]any{"chat_id": message.ExternalID, "text": message.Text})
}
func (s *Service) slackSend(ctx context.Context, account domain.ChannelAccount, message domain.ChannelMessage) error {
	token := s.secret(account, "bot_token")
	if token == "" {
		return fmt.Errorf("slack bot_token secret ref is unset")
	}
	return s.postJSON(ctx, "https://slack.com/api/chat.postMessage", map[string]string{"Authorization": "Bearer " + token}, map[string]any{"channel": message.ExternalID, "text": message.Text})
}
func (s *Service) discordSend(ctx context.Context, account domain.ChannelAccount, message domain.ChannelMessage) error {
	route := route(message.Attachments)
	if token, ok := route["interaction_token"].(string); ok {
		appID, _ := route["application_id"].(string)
		return s.postJSON(ctx, "https://discord.com/api/v10/webhooks/"+appID+"/"+token, nil, map[string]any{"content": message.Text})
	}
	bot := s.secret(account, "bot_token")
	channelID := message.ExternalID
	if value, ok := route["channel_id"].(string); ok {
		channelID = value
	}
	if bot == "" || channelID == "" {
		return fmt.Errorf("discord reply route unavailable")
	}
	return s.postJSON(ctx, "https://discord.com/api/v10/channels/"+channelID+"/messages", map[string]string{"Authorization": "Bot " + bot}, map[string]any{"content": message.Text})
}
func (s *Service) whatsappSend(ctx context.Context, account domain.ChannelAccount, message domain.ChannelMessage) error {
	token := s.secret(account, "access_token")
	phone, _ := stringSetting(account.Settings, "phone_number_id")
	if token == "" || phone == "" {
		return fmt.Errorf("whatsapp access_token or phone_number_id unavailable")
	}
	return s.postJSON(ctx, "https://graph.facebook.com/v22.0/"+phone+"/messages", map[string]string{"Authorization": "Bearer " + token}, map[string]any{"messaging_product": "whatsapp", "to": message.ExternalID, "type": "text", "text": map[string]string{"body": message.Text}})
}
func (s *Service) postJSON(ctx context.Context, url string, headers map[string]string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("channel API status %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (s *Service) secret(account domain.ChannelAccount, key string) string {
	ref := account.SecretRefs[key]
	if !validEnvRef(ref) {
		return ""
	}
	return strings.TrimSpace(os.Getenv(ref))
}
func readBody(r *http.Request) ([]byte, error) { return io.ReadAll(io.LimitReader(r.Body, 2<<20)) }
func validSlack(secret, timestamp, signature string, body []byte) bool {
	if secret == "" {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(seconds, 0)) > 5*time.Minute {
		return false
	}
	return validHMAC(secret, signature, []byte("v0:"+timestamp+":"+string(body)), "v0=")
}
func validHMAC(secret, signature string, body []byte, prefix string) bool {
	if secret == "" || !strings.HasPrefix(signature, prefix) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}
func validDiscord(public, signature, timestamp string, body []byte) bool {
	key, err := hex.DecodeString(public)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(key), append([]byte(timestamp), body...), sig)
}
func routeAttachment(value inbound) []map[string]any {
	if len(value.Attachments) == 0 {
		return []map[string]any{{"channel_id": value.ReplyID}}
	}
	copy := map[string]any{"channel_id": value.ReplyID}
	for key, item := range value.Attachments[0] {
		copy[key] = item
	}
	return []map[string]any{copy}
}
func route(values []map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	return values[0]
}
func trimSummary(value string) string {
	if len(value) > 12000 {
		return value[len(value)-12000:]
	}
	return value
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func randomID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		hash := sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano())))
		return hex.EncodeToString(hash[:16])
	}
	return hex.EncodeToString(raw)
}
func validEnvRef(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if !(r == '_' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func stringSetting(values map[string]any, key string) (string, bool) {
	value, ok := values[key].(string)
	return strings.TrimSpace(value), ok && strings.TrimSpace(value) != ""
}

func (s *Service) startDiscordGateways(ctx context.Context) {
	accounts, err := s.store.ChannelAccounts(ctx)
	if err != nil {
		return
	}
	for _, account := range accounts {
		if account.Adapter != "discord" || !account.Enabled {
			continue
		}
		token := s.secret(account, "bot_token")
		if token == "" {
			continue
		}
		child, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		if _, exists := s.gateways[account.ID]; !exists {
			s.gateways[account.ID] = cancel
			go s.discordGateway(child, account, token)
		} else {
			cancel()
		}
		s.mu.Unlock()
	}
}
func (s *Service) discordGateway(ctx context.Context, account domain.ChannelAccount, token string) {
	for ctx.Err() == nil {
		connection, _, err := websocket.DefaultDialer.DialContext(ctx, "wss://gateway.discord.gg/?v=10&encoding=json", nil)
		if err != nil {
			sleep(ctx, time.Second)
			continue
		}
		var hello struct {
			Op int `json:"op"`
			D  struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			} `json:"d"`
		}
		if err := connection.ReadJSON(&hello); err != nil {
			connection.Close()
			sleep(ctx, time.Second)
			continue
		}
		_ = connection.WriteJSON(map[string]any{"op": 2, "d": map[string]any{"token": token, "intents": 33281, "properties": map[string]string{"os": "norbot", "browser": "norbot", "device": "norbot"}}})
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Duration(hello.D.HeartbeatInterval) * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					_ = connection.WriteJSON(map[string]any{"op": 1, "d": nil})
				}
			}
		}()
		for {
			var event struct {
				Op int             `json:"op"`
				T  string          `json:"t"`
				D  json.RawMessage `json:"d"`
			}
			if err := connection.ReadJSON(&event); err != nil {
				break
			}
			if event.T != "MESSAGE_CREATE" {
				continue
			}
			var message struct {
				ID        string `json:"id"`
				Content   string `json:"content"`
				ChannelID string `json:"channel_id"`
				Author    struct {
					ID  string `json:"id"`
					Bot bool   `json:"bot"`
				} `json:"author"`
				Attachments []map[string]any `json:"attachments"`
			}
			if json.Unmarshal(event.D, &message) == nil && !message.Author.Bot && message.Author.ID != "" {
				s.acceptAsync(account, inbound{PlatformID: message.ID, ExternalID: message.Author.ID, ReplyID: message.ChannelID, Text: message.Content, Attachments: message.Attachments})
			}
		}
		close(done)
		connection.Close()
		sleep(ctx, time.Second)
	}
}
func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
