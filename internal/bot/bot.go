package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/daddymaou/dart-otp/internal/config"
	"github.com/daddymaou/dart-otp/internal/db"
	"github.com/daddymaou/dart-otp/internal/phone"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/telebot.v3"
)

type Bot struct {
	bot           *telebot.Bot
	pool          *pgxpool.Pool
	cfg           *config.Config
	pending       *PendingLinks
	sender        *Sender
	onDeliverWaiting func(tenantID, phone string, chatID int64)
}

func (bt *Bot) SetDeliverWaiting(f func(tenantID, phone string, chatID int64)) {
	bt.onDeliverWaiting = f
}

func (bt *Bot) Sender() *Sender { return bt.sender }

func Start(cfg *config.Config, pool *pgxpool.Pool) (*Bot, error) {
	pref := telebot.Settings{
		Token:  cfg.TelegramBotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}
	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("telebot: %w", err)
	}
	bot := &Bot{
		bot:     b,
		pool:    pool,
		cfg:     cfg,
		pending: NewPendingLinks(),
		sender:  NewSender(b),
	}
	bot.registerHandlers()
	go b.Start()
	return bot, nil
}

func (bt *Bot) Stop() {
	bt.bot.Stop()
}

func (bt *Bot) registerHandlers() {
	bt.bot.Handle("/start", bt.handleStart)
	bt.bot.Handle(telebot.OnContact, bt.handleContact)
	bt.bot.Handle("/stop", bt.handleStop)
	bt.bot.Handle("/help", bt.handleHelp)
}

func (bt *Bot) handleStart(c telebot.Context) error {
	payload := strings.TrimSpace(strings.TrimPrefix(c.Message().Payload, " "))
	if payload == "" {
		return c.Send("This bot delivers verification codes. Codes are sent here after a service asks you to link your number.")
	}
	hash := HashToken(payload)
	lt, err := db.GetLinkToken(context.Background(), bt.pool, hash)
	if err != nil || lt.UsedAt != nil || time.Now().After(lt.ExpiresAt) {
		return c.Send("This link has expired. Please request a new code from the app.")
	}
	bt.pending.Set(c.Sender().ID, hash, 15*time.Minute)
	return c.Send("Tap the button below to share your number.", &telebot.SendOptions{
		ReplyMarkup: &telebot.ReplyMarkup{
			ResizeKeyboard: true,
			OneTimeKeyboard: true,
			ReplyKeyboard: []telebot.ReplyButton{
				{Text: "Share my number", Contact: true},
			},
		},
	})
}

func (bt *Bot) handleContact(c telebot.Context) error {
	msg := c.Message()
	if msg.Contact == nil {
		return nil
	}
	normalized, err := ValidateContact(msg.Sender.ID, msg.Contact.UserID, msg.Contact.PhoneNumber, "")
	if err != nil {
		return c.Send(err.Error())
	}
	tokenHash, ok := bt.pending.Get(msg.Sender.ID)
	if !ok {
		return c.Send(ErrTokenExpired.Error())
	}
	lt, err := db.GetLinkToken(context.Background(), bt.pool, tokenHash)
	if err != nil || lt.UsedAt != nil || time.Now().After(lt.ExpiresAt) {
		bt.pending.Delete(msg.Sender.ID)
		return c.Send(ErrTokenExpired.Error())
	}
	tokenPhone, err := phone.Normalize(lt.Phone)
	if err != nil {
		return c.Send(ErrTokenExpired.Error())
	}
	if err := CheckPhoneMatch(normalized, tokenPhone); err != nil {
		return c.Send(err.Error())
	}
	tx, err := bt.pool.Begin(context.Background())
	if err != nil {
		return c.Send(deliveryFailedMsg)
	}
	defer tx.Rollback(context.Background())
	if err := db.UpsertTelegramLink(context.Background(), tx, tokenPhone, msg.Sender.ID, msg.Contact.UserID); err != nil {
		return c.Send(deliveryFailedMsg)
	}
	if err := db.MarkLinkTokenUsed(context.Background(), tx, tokenHash); err != nil {
		return c.Send(deliveryFailedMsg)
	}
	if err := tx.Commit(context.Background()); err != nil {
		return c.Send(deliveryFailedMsg)
	}
	bt.pending.Delete(msg.Sender.ID)
	_ = c.Send("Linked. You will receive your codes here.", &telebot.SendOptions{ReplyMarkup: &telebot.ReplyMarkup{RemoveKeyboard: true}})
	bt.deliverWaitingCode(lt.TenantID, tokenPhone, msg.Sender.ID)
	return nil
}

func (bt *Bot) handleStop(c telebot.Context) error {
	if err := db.SetLinksInactiveForChat(context.Background(), bt.pool, c.Sender().ID); err != nil {
		return c.Send(deliveryFailedMsg)
	}
	return c.Send("Unlinked. You will no longer receive codes.")
}

func (bt *Bot) handleHelp(c telebot.Context) error {
	return c.Send("This bot delivers verification codes. Start with /start, or open the deep link sent to you by the app you are signing into.")
}

var ErrBlockedSender = errors.New("telegram_blocked")

const deliveryFailedMsg = "Something went wrong delivering your code. Please request a new one."

// deliverWaitingCode sends the OTP that was blocked on linking; implemented with the OTP core in step 5.
func (bt *Bot) deliverWaitingCode(tenantID, phone string, chatID int64) {
	if bt.onDeliverWaiting != nil {
		bt.onDeliverWaiting(tenantID, phone, chatID)
	}
}
