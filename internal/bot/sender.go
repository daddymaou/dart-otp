package bot

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"gopkg.in/telebot.v3"
)

var ErrBlocked = errors.New("telegram_blocked")

type Sender struct {
	bot *telebot.Bot
}

func NewSender(b *telebot.Bot) *Sender { return &Sender{bot: b} }

var codePlaceholder = regexp.MustCompile(`\{\{code\}\}`)

func RenderBody(body, code string) string {
	escaped := html.EscapeString(body)
	return codePlaceholder.ReplaceAllString(escaped, "<code>"+code+"</code>")
}

func (s *Sender) Send(chatID int64, renderedBody string) error {
	chat := &telebot.Chat{ID: chatID}
	if _, err := s.bot.Send(chat, renderedBody, &telebot.SendOptions{ParseMode: telebot.ModeHTML}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") ||
			strings.Contains(strings.ToLower(err.Error()), "bot was blocked by the user") {
			return ErrBlocked
		}
		return fmt.Errorf("telegram send: %w", err)
	}
	return nil
}
