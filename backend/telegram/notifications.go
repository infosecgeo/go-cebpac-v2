package telegram

import (
"context"
"fmt"
"strconv"
"time"

"cebupac/backend/database"

tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SendTransactionNotification sends a notification about a successful transaction
func (b *Bot) SendTransactionNotification(ctx context.Context, transaction *database.Transaction, itineraryDetails string) error {
// Get notification channel from settings
settingsRepo, err := b.db.AdminSettings()
if err != nil {
return fmt.Errorf("get settings: %w", err)
}

settings, err := settingsRepo.List(ctx)
if err != nil || len(settings) == 0 {
return fmt.Errorf("no settings found")
}

notificationChannel := settings[0].NotificationChannel
if notificationChannel == "" {
b.logger.Warn("Notification channel not configured", nil)
return fmt.Errorf("notification channel not configured")
}

channelID, err := strconv.ParseInt(notificationChannel, 10, 64)
if err != nil {
return fmt.Errorf("invalid channel ID: %w", err)
}

// Get user information
usersRepo, err := b.db.Users()
if err != nil {
return fmt.Errorf("get users repo: %w", err)
}

user, found, err := usersRepo.Get(ctx, transaction.UserID)
if err != nil || !found {
return fmt.Errorf("user not found")
}

// Convert to Philippine Time (UTC+8)
phTime := transaction.Timestamp.Add(8 * time.Hour)

// Format notification message with all unmasked data
text := fmt.Sprintf(`🎉 <b>Successful Transaction</b>

<b>Transaction Details:</b>
Transaction ID: %s
Record Locator: %s
Date: %s
Time: %s (Philippine Time)

<b>User Information:</b>
Email: %s
License: %s
User ID: %s

<b>Payment Details:</b>
Card (Last 4): %s
Amount: ₱%d
Status: %s

<b>Booking Information:</b>
%s

<b>Additional Details:</b>
Loc Code: %s
Loc Sub Code: %s
Fraud Status: %s`,
transaction.ID,
transaction.RecordLocator,
phTime.Format("01/02/2006"),
phTime.Format("15:04:05"),
user.Email,
user.LicenseKey,
user.ID,
transaction.CardLast4,
transaction.Amount,
transaction.Status,
itineraryDetails,
transaction.LocCode,
transaction.LocSubCode,
transaction.FraudStatus,
)

err = b.SendMessage(channelID, text)
if err != nil {
b.logger.Error("Failed to send transaction notification", map[string]string{
"error":          err.Error(),
"transaction_id": transaction.ID,
})
return err
}

b.logger.Info("Transaction notification sent", map[string]string{
"transaction_id": transaction.ID,
"channel_id":     notificationChannel,
})

return nil
}

// handlePhotoUpload processes payment receipt uploads
func (b *Bot) handlePhotoUpload(message *tgbotapi.Message) {
chatID := message.Chat.ID
telegramID := message.From.ID

ctx := context.Background()

// Get user
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

// Find pending topup request for this user
topupRepo, err := b.db.TopupRequests()
if err != nil {
b.SendMessage(chatID, "❌ Database error. Please try again later.")
return
}

pendingTopup, found, err := topupRepo.FindOne(ctx, func(t database.TopupRequest) bool {
return t.TelegramID == telegramID && t.Status == database.TopupStatusPending && t.PaymentReceiptURL == ""
})

if err != nil || !found {
b.SendMessage(chatID, "❌ No pending top-up request found. Please use /topup first.")
return
}

// Get the largest photo (best quality)
photo := message.Photo[len(message.Photo)-1]
fileURL, err := b.api.GetFileDirectURL(photo.FileID)
if err != nil {
b.SendMessage(chatID, "❌ Failed to process photo. Please try again.")
return
}

// Update topup request with receipt URL
pendingTopup.PaymentReceiptURL = fileURL
pendingTopup.TelegramMessageID = message.MessageID
pendingTopup.UpdatedAt = time.Now()

err = topupRepo.Upsert(ctx, pendingTopup)
if err != nil {
b.SendMessage(chatID, "❌ Failed to save receipt. Please try again.")
return
}

// Send to admin approval channel
err = b.sendApprovalRequest(ctx, &pendingTopup, &user)
if err != nil {
b.logger.Error("Failed to send approval request", map[string]string{
"error": err.Error(),
"topup_id": pendingTopup.ID,
})
b.SendMessage(chatID, "⚠️ Receipt uploaded but failed to notify admin. Please contact support.")
return
}

text := fmt.Sprintf(`✅ <b>Payment Receipt Uploaded</b>

Request ID: %s
Amount: %d credits

Your top-up request has been submitted for admin approval. You will be notified once it's processed.

Thank you for your patience!`, pendingTopup.ID, pendingTopup.Amount)

b.SendMessage(chatID, text)

b.logger.Info("Payment receipt uploaded", map[string]string{
"user_id":  user.ID,
"topup_id": pendingTopup.ID,
"amount":   fmt.Sprintf("%d", pendingTopup.Amount),
})
}

// sendApprovalRequest sends topup request to admin channel for approval
func (b *Bot) sendApprovalRequest(ctx context.Context, topup *database.TopupRequest, user *database.User) error {
// Get approval channel from settings
settingsRepo, err := b.db.AdminSettings()
if err != nil {
return fmt.Errorf("get settings: %w", err)
}

settings, err := settingsRepo.List(ctx)
if err != nil || len(settings) == 0 {
return fmt.Errorf("no settings found")
}

approvalChannel := settings[0].ApprovalChannel
if approvalChannel == "" {
return fmt.Errorf("approval channel not configured")
}

channelID, err := strconv.ParseInt(approvalChannel, 10, 64)
if err != nil {
return fmt.Errorf("invalid channel ID: %w", err)
}

text := fmt.Sprintf(`💰 <b>New Top-Up Request</b>

<b>User Information:</b>
Username: %s
License: %s
Telegram: @%s
User ID: %s

<b>Request Details:</b>
Amount: %d credits
Request ID: %s
Created: %s

<b>Payment Receipt:</b>
Receipt attached below.`,
user.Username,
user.LicenseKey,
user.TelegramUsername,
user.ID,
topup.Amount,
topup.ID,
topup.CreatedAt.Format("2006-01-02 15:04:05"),
)

// Send photo with inline buttons
if topup.PaymentReceiptURL != "" {
msg := tgbotapi.NewPhoto(channelID, tgbotapi.FileURL(topup.PaymentReceiptURL))
msg.Caption = text
msg.ParseMode = "HTML"

// Add inline keyboard for approval/denial
keyboard := tgbotapi.NewInlineKeyboardMarkup(
tgbotapi.NewInlineKeyboardRow(
tgbotapi.NewInlineKeyboardButtonData("✅ Approve", "approve_"+topup.ID),
tgbotapi.NewInlineKeyboardButtonData("❌ Deny", "deny_"+topup.ID),
),
)
msg.ReplyMarkup = keyboard

sentMsg, err := b.api.Send(msg)
if err != nil {
return err
}

// Update topup with admin message ID
topup.AdminChannelMessageID = sentMsg.MessageID
topupRepo, _ := b.db.TopupRequests()
topupRepo.Upsert(ctx, *topup)
}

return nil
}
