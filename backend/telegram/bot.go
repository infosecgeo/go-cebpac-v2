package telegram

import (
"context"
"fmt"
"sync"

"cebupac/backend/config"
"cebupac/backend/database"
"cebupac/backend/logger"

tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot instance
type Bot struct {
api          *tgbotapi.BotAPI
db           *database.Database
cfg          *config.Config
logger       *logger.Logger
updatesChan  tgbotapi.UpdatesChannel
stopChan     chan struct{}
wg           sync.WaitGroup
mu           sync.RWMutex
isRunning    bool
}

var (
botInstance *Bot
botOnce     sync.Once
botErr      error
)

// GetBot returns the singleton Bot instance
func GetBot() (*Bot, error) {
botOnce.Do(func() {
botInstance, botErr = NewBot()
})
return botInstance, botErr
}

// NewBot creates a new Telegram bot instance
func NewBot() (*Bot, error) {
cfg := config.GetConfig()
db := database.GetDatabase()
log := logger.GetLogger()

// Get bot token from admin settings or config
botToken := getBotToken()
if botToken == "" {
log.Warn("Telegram bot token not configured", nil)
return nil, fmt.Errorf("telegram bot token not configured")
}

api, err := tgbotapi.NewBotAPI(botToken)
if err != nil {
log.Error("Failed to create Telegram bot", map[string]string{"error": err.Error()})
return nil, fmt.Errorf("create telegram bot: %w", err)
}

// Set debug mode based on environment
api.Debug = cfg.Server.Environment == "development"

bot := &Bot{
api:       api,
db:        db,
cfg:       cfg,
logger:    log,
stopChan:  make(chan struct{}),
isRunning: false,
}

log.Info("Telegram bot initialized", map[string]string{
"username": api.Self.UserName,
})

return bot, nil
}

// Start begins processing Telegram updates
func (b *Bot) Start() error {
b.mu.Lock()
if b.isRunning {
b.mu.Unlock()
return fmt.Errorf("bot is already running")
}
b.isRunning = true
b.mu.Unlock()

u := tgbotapi.NewUpdate(0)
u.Timeout = 60

b.updatesChan = b.api.GetUpdatesChan(u)

b.wg.Add(1)
go b.processUpdates()

b.logger.Info("Telegram bot started", nil)
return nil
}

// Stop gracefully stops the bot
func (b *Bot) Stop(ctx context.Context) error {
b.mu.Lock()
if !b.isRunning {
b.mu.Unlock()
return nil
}
b.isRunning = false
b.mu.Unlock()

close(b.stopChan)

// Wait for graceful shutdown or context timeout
done := make(chan struct{})
go func() {
b.wg.Wait()
close(done)
}()

select {
case <-done:
b.logger.Info("Telegram bot stopped gracefully", nil)
case <-ctx.Done():
b.logger.Warn("Telegram bot shutdown timed out", nil)
return ctx.Err()
}

b.api.StopReceivingUpdates()
return nil
}

// processUpdates handles incoming Telegram updates
func (b *Bot) processUpdates() {
defer b.wg.Done()

for {
select {
case <-b.stopChan:
return
case update := <-b.updatesChan:
go b.handleUpdate(update)
}
}
}

// handleUpdate routes updates to appropriate handlers
func (b *Bot) handleUpdate(update tgbotapi.Update) {
defer func() {
if r := recover(); r != nil {
b.logger.Error("Panic in Telegram update handler", map[string]string{
"error": fmt.Sprintf("%v", r),
})
}
}()

// Handle messages
if update.Message != nil {
b.handleMessage(update.Message)
return
}

// Handle callback queries (inline buttons)
if update.CallbackQuery != nil {
b.handleCallbackQuery(update.CallbackQuery)
return
}
}

// getBotToken retrieves the bot token from admin settings or environment
func getBotToken() string {
// Try to get from admin settings first
db := database.GetDatabase()
settingsRepo, err := db.AdminSettings()
if err == nil {
settings, err := settingsRepo.List(context.Background())
if err == nil && len(settings) > 0 && settings[0].TelegramBotToken != "" {
return settings[0].TelegramBotToken
}
}

// Fall back to environment variable
// In production, should be set via environment
return ""
}

// SendMessage sends a text message to a chat
func (b *Bot) SendMessage(chatID int64, text string) error {
msg := tgbotapi.NewMessage(chatID, text)
msg.ParseMode = "HTML"
_, err := b.api.Send(msg)
if err != nil {
b.logger.Error("Failed to send Telegram message", map[string]string{
"error":   err.Error(),
"chat_id": fmt.Sprintf("%d", chatID),
})
}
return err
}

// SendPhoto sends a photo to a chat
func (b *Bot) SendPhoto(chatID int64, photoPath string, caption string) error {
msg := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(photoPath))
msg.Caption = caption
msg.ParseMode = "HTML"
_, err := b.api.Send(msg)
if err != nil {
b.logger.Error("Failed to send Telegram photo", map[string]string{
"error":   err.Error(),
"chat_id": fmt.Sprintf("%d", chatID),
})
}
return err
}

// IsRunning returns whether the bot is currently running
func (b *Bot) IsRunning() bool {
b.mu.RLock()
defer b.mu.RUnlock()
return b.isRunning
}
