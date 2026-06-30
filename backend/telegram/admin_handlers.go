package telegram

import (
"context"
"fmt"
"strings"
"time"

"cebupac/backend/database"

tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCallbackQuery processes inline button callbacks
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
data := query.Data

if strings.HasPrefix(data, "approve_") {
b.handleApproval(query, true)
} else if strings.HasPrefix(data, "deny_") {
b.handleApproval(query, false)
}
}

// handleApproval processes topup approval/denial
func (b *Bot) handleApproval(query *tgbotapi.CallbackQuery, approve bool) {
topupID := strings.TrimPrefix(query.Data, "approve_")
if !approve {
topupID = strings.TrimPrefix(query.Data, "deny_")
}

ctx := context.Background()
topupRepo, err := b.db.TopupRequests()
if err != nil {
b.answerCallback(query, "❌ Database error")
return
}

// Get topup request
topup, found, err := topupRepo.Get(ctx, topupID)
if err != nil || !found {
b.answerCallback(query, "❌ Top-up request not found")
return
}

// Check if already processed
if topup.Status != database.TopupStatusPending {
b.answerCallback(query, fmt.Sprintf("⚠️ This request was already %s", topup.Status))
return
}

// Update status
if approve {
topup.Status = database.TopupStatusApproved
} else {
topup.Status = database.TopupStatusDenied
}
topup.ProcessedAt = time.Now()
topup.ProcessedBy = query.From.UserName
topup.UpdatedAt = time.Now()

err = topupRepo.Upsert(ctx, topup)
if err != nil {
b.answerCallback(query, "❌ Failed to update status")
return
}

// If approved, add credits to user
if approve {
usersRepo, err := b.db.Users()
if err != nil {
b.answerCallback(query, "❌ Failed to add credits")
return
}

user, found, err := usersRepo.Get(ctx, topup.UserID)
if err != nil || !found {
b.answerCallback(query, "❌ User not found")
return
}

user.Credits += topup.Amount
err = usersRepo.Upsert(ctx, user)
if err != nil {
b.answerCallback(query, "❌ Failed to add credits")
return
}

// Notify user of approval
notifyText := fmt.Sprintf(`✅ <b>Top-Up Approved!</b>

Your top-up request has been approved.

<b>Details:</b>
Amount: %d credits
New Balance: %d credits
Request ID: %s

Your credits have been added to your account. You can now use the payment processor!`,
topup.Amount,
user.Credits,
topup.ID,
)

b.SendMessage(user.TelegramID, notifyText)

b.logger.Info("Top-up approved", map[string]string{
"user_id":      user.ID,
"topup_id":     topup.ID,
"amount":       fmt.Sprintf("%d", topup.Amount),
"processed_by": query.From.UserName,
})
} else {
// Notify user of denial
usersRepo, _ := b.db.Users()
user, found, _ := usersRepo.Get(ctx, topup.UserID)
if found {
notifyText := fmt.Sprintf(`❌ <b>Top-Up Denied</b>

Unfortunately, your top-up request has been denied.

<b>Details:</b>
Amount: %d credits
Request ID: %s

Please contact support for more information or submit a new request with valid payment proof.`,
topup.Amount,
topup.ID,
)

b.SendMessage(user.TelegramID, notifyText)
}

b.logger.Info("Top-up denied", map[string]string{
"topup_id":     topup.ID,
"processed_by": query.From.UserName,
})
}

// Update the admin message
statusEmoji := "❌"
statusText := "DENIED"
if approve {
statusEmoji = "✅"
statusText = "APPROVED"
}

editText := fmt.Sprintf("%s <b>%s</b>\n\n%s\n\nProcessed by: @%s\nTime: %s",
statusEmoji,
statusText,
query.Message.Caption,
query.From.UserName,
time.Now().Format("2006-01-02 15:04:05"),
)

editMsg := tgbotapi.NewEditMessageCaption(query.Message.Chat.ID, query.Message.MessageID, editText)
editMsg.ParseMode = "HTML"
b.api.Send(editMsg)

// Answer callback
if approve {
b.answerCallback(query, "✅ Top-up approved and credits added!")
} else {
b.answerCallback(query, "❌ Top-up denied")
}
}

// answerCallback sends a callback answer
func (b *Bot) answerCallback(query *tgbotapi.CallbackQuery, text string) {
callback := tgbotapi.NewCallback(query.ID, text)
b.api.Request(callback)
}
