package telegram

import (
"context"
"fmt"
"strconv"
"strings"
"time"

"cebupac/backend/database"

tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
"github.com/google/uuid"
)

// handleMessage processes incoming text messages
func (b *Bot) handleMessage(message *tgbotapi.Message) {
if message.IsCommand() {
b.handleCommand(message)
return
}

// Handle photo uploads (payment receipts)
if message.Photo != nil {
b.handlePhotoUpload(message)
return
}

// Default response for non-command messages
b.SendMessage(message.Chat.ID, "Please use /start to see available commands.")
}

// handleCommand routes commands to specific handlers
func (b *Bot) handleCommand(message *tgbotapi.Message) {
command := message.Command()
args := message.CommandArguments()

switch command {
case "start":
b.handleStartCommand(message)
case "link":
b.handleLinkCommand(message, args)
case "credits":
b.handleCreditsCommand(message)
case "topup":
b.handleTopupCommand(message, args)
case "status":
b.handleStatusCommand(message)
case "help":
b.handleHelpCommand(message)
default:
b.SendMessage(message.Chat.ID, "Unknown command. Use /help to see available commands.")
}
}

// handleStartCommand sends welcome message
func (b *Bot) handleStartCommand(message *tgbotapi.Message) {
text := `<b>🚀 Welcome to CebuPacific Payment Processor Bot!</b>

This bot helps you manage your license, credits, and top-ups.

<b>Available Commands:</b>
/link &lt;license_key&gt; - Link your license to this Telegram account
/credits - Check your current credit balance
/topup &lt;amount&gt; - Request credit top-up
/status - Check your account status
/help - Show this help message

<b>Get Started:</b>
1. Use /link to connect your license
2. Top up credits using /topup
3. Start using the payment processor!

Need help? Contact support.`

b.SendMessage(message.Chat.ID, text)
}

// handleLinkCommand links a license to a Telegram account
func (b *Bot) handleLinkCommand(message *tgbotapi.Message, args string) {
chatID := message.Chat.ID
telegramID := message.From.ID
telegramUsername := message.From.UserName

if args == "" {
b.SendMessage(chatID, "❌ Please provide a license key.\n\nUsage: /link YOUR_LICENSE_KEY")
return
}

licenseKey := strings.TrimSpace(args)

// Get license from database
ctx := context.Background()
licensesRepo, err := b.db.Licenses()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

license, found, err := licensesRepo.FindOne(ctx, func(l database.License) bool {
return l.Key == licenseKey
})
if err != nil || !found {
b.SendMessage(chatID, "❌ Invalid license key. Please check and try again.")
return
}

// Check if license is already linked
if license.LinkedTelegramID != 0 && license.LinkedTelegramID != telegramID {
b.SendMessage(chatID, "❌ This license is already linked to another Telegram account.")
return
}

// Check if this Telegram account is already linked to another license
existingLicense, found, err := licensesRepo.FindOne(ctx, func(l database.License) bool {
return l.LinkedTelegramID == telegramID
})
if err == nil && found && existingLicense.Key != licenseKey {
b.SendMessage(chatID, fmt.Sprintf("❌ Your Telegram account is already linked to license: %s\n\nPlease unlink first or contact support.", maskLicense(existingLicense.Key)))
return
}

// Link the license
license.LinkedTelegramID = telegramID
err = licensesRepo.Upsert(ctx, license)
if err != nil {
b.SendMessage(chatID, "❌ Failed to link license. Please try again.")
return
}

// Update user with Telegram info
usersRepo, err := b.db.Users()
if err == nil {
user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
return u.LicenseKey == licenseKey
})
if err == nil && found {
user.TelegramID = telegramID
user.TelegramUsername = telegramUsername
usersRepo.Upsert(ctx, user)
}
}

b.logger.Info("License linked to Telegram", map[string]string{
"license_key":  maskLicense(licenseKey),
"telegram_id":  fmt.Sprintf("%d", telegramID),
"telegram_username": telegramUsername,
})

text := fmt.Sprintf(`✅ <b>License Linked Successfully!</b>

License: %s
Telegram: @%s

You can now use all bot features. Check your balance with /credits.`, maskLicense(licenseKey), telegramUsername)

b.SendMessage(chatID, text)
}

// handleCreditsCommand shows user's credit balance
func (b *Bot) handleCreditsCommand(message *tgbotapi.Message) {
chatID := message.Chat.ID
telegramID := message.From.ID

ctx := context.Background()
usersRepo, err := b.db.Users()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
return u.TelegramID == telegramID
})

if err != nil || !found {
b.SendMessage(chatID, "❌ No account found. Please link your license first using /link")
return
}

text := fmt.Sprintf(`💰 <b>Credit Balance</b>

Current Credits: <b>%d</b>
License: %s
Status: %s

Use /topup to add more credits.`, user.Credits, maskLicense(user.LicenseKey), user.Status)

b.SendMessage(chatID, text)
}

// handleTopupCommand initiates a top-up request
func (b *Bot) handleTopupCommand(message *tgbotapi.Message, args string) {
chatID := message.Chat.ID
telegramID := message.From.ID

if args == "" {
b.SendMessage(chatID, "❌ Please specify the amount.\n\nUsage: /topup AMOUNT\n\nExample: /topup 10")
return
}

amount, err := strconv.Atoi(strings.TrimSpace(args))
if err != nil || amount <= 0 {
b.SendMessage(chatID, "❌ Invalid amount. Please enter a positive number.")
return
}

ctx := context.Background()
usersRepo, err := b.db.Users()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
return u.TelegramID == telegramID
})

if err != nil || !found {
b.SendMessage(chatID, "❌ No account found. Please link your license first using /link")
return
}

// Create topup request
topupRepo, err := b.db.TopupRequests()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

topupRequest := database.TopupRequest{
ID:         "topup_" + uuid.New().String(),
UserID:     user.ID,
TelegramID: telegramID,
Amount:     amount,
Status:     database.TopupStatusPending,
CreatedAt:  time.Now(),
UpdatedAt:  time.Now(),
}

err = topupRepo.Create(ctx, topupRequest)
if err != nil {
b.SendMessage(chatID, "❌ Failed to create top-up request. Please try again.")
return
}

// Get QR code and payment instructions
settingsRepo, err := b.db.AdminSettings()
var qrCodePath string
var instructions string
if err == nil {
settings, err := settingsRepo.List(ctx)
if err == nil && len(settings) > 0 {
qrCodePath = settings[0].QRCodeImage
instructions = settings[0].PaymentInstructions
}
}

// Send QR code and instructions
text := fmt.Sprintf(`💳 <b>Top-Up Request Created</b>

Amount: <b>%d credits</b>
Request ID: %s

<b>Payment Instructions:</b>
%s

Please upload your payment receipt as a photo to complete the request.`, amount, topupRequest.ID, instructions)

if qrCodePath != "" {
b.SendPhoto(chatID, qrCodePath, text)
} else {
b.SendMessage(chatID, text)
}

b.logger.Info("Top-up request created", map[string]string{
"user_id":     user.ID,
"telegram_id": fmt.Sprintf("%d", telegramID),
"amount":      fmt.Sprintf("%d", amount),
"request_id":  topupRequest.ID,
})
}

// handleStatusCommand shows user's account status
func (b *Bot) handleStatusCommand(message *tgbotapi.Message) {
chatID := message.Chat.ID
telegramID := message.From.ID

ctx := context.Background()
usersRepo, err := b.db.Users()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

user, found, err := usersRepo.FindOne(ctx, func(u database.User) bool {
return u.TelegramID == telegramID
})

if err != nil || !found {
b.SendMessage(chatID, "❌ No account found. Please link your license first using /link")
return
}

// Get license info
licensesRepo, err := b.db.Licenses()
var licenseStatus string
var licenseExpiry string
if err == nil {
license, found, err := licensesRepo.FindOne(ctx, func(l database.License) bool {
return l.Key == user.LicenseKey
})
if err == nil && found {
licenseStatus = license.Status
licenseExpiry = license.ExpiresAt.Format("2006-01-02")
}
}

text := fmt.Sprintf(`📊 <b>Account Status</b>

<b>User Information:</b>
Username: %s
License: %s
Credits: %d

<b>License Information:</b>
Status: %s
Expires: %s

<b>Account Status:</b>
Status: %s
Registered: %s
Last Login: %s

Use /credits to check balance or /topup to add credits.`, 
user.Username, 
maskLicense(user.LicenseKey), 
user.Credits,
licenseStatus,
licenseExpiry,
user.Status,
user.CreatedAt.Format("2006-01-02"),
user.LastLogin.Format("2006-01-02 15:04"))

b.SendMessage(chatID, text)
}

// handleHelpCommand shows help information
func (b *Bot) handleHelpCommand(message *tgbotapi.Message) {
b.handleStartCommand(message)
}

// maskLicense masks license key for display
func maskLicense(key string) string {
if len(key) <= 8 {
return "****"
}
return key[:4] + "****" + key[len(key)-4:]
}
