package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/user/golovebox/internal/agent"
)

// triggerRe matches "issue #42" or "issue #42 repo owner/repo" (case-insensitive).
var triggerRe = regexp.MustCompile(`(?i)issue\s+#?(\d+)(?:\s+repo\s+([0-9A-Za-z._-]+/[0-9A-Za-z._-]+))?`)

// TelegramHandler listens for messages and dispatches agent tasks.
type TelegramHandler struct {
	bot *tgbotapi.BotAPI
	gw  *Gateway
}

// NewTelegramHandler creates a TelegramHandler and verifies the bot token.
func NewTelegramHandler(token string, gw *Gateway) (*TelegramHandler, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	return &TelegramHandler{bot: bot, gw: gw}, nil
}

// Start begins polling for updates and blocks until ctx is cancelled.
func (h *TelegramHandler) Start(ctx context.Context) error {
	slog.Info("telegram bot online", "username", h.bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := h.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil {
				continue
			}
			go h.handleMessage(ctx, update.Message)
		}
	}
}

// Stop shuts down the update poller.
func (h *TelegramHandler) Stop() error {
	h.bot.StopReceivingUpdates()
	return nil
}

func (h *TelegramHandler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	lower := strings.ToLower(text)

	switch {
	case lower == "help":
		h.send(msg.Chat.ID,
			"Commands:\n"+
				"  issue #<N> [repo owner/repo] — resolve a GitHub issue\n"+
				"  status — show VM status\n"+
				"  help — this message")
		return

	case lower == "status":
		h.send(msg.Chat.ID, "VM: running")
		return
	}

	owner, repo, issueNum, ok := parseTrigger(text, h.gw.cfg.DefaultRepo)
	if !ok {
		h.send(msg.Chat.ID, "Usage: issue #<N> [repo owner/repo]")
		return
	}

	h.send(msg.Chat.ID, fmt.Sprintf("⚙️ Processando issue #%d em %s/%s...", issueNum, owner, repo))

	chatID := msg.Chat.ID
	progress := func(iter int, action, _, obs string) {
		h.send(chatID, fmt.Sprintf("🔄 [%d/%d] %s: %s", iter, agent.MaxIterations, action, obs))
	}

	result, err := h.gw.RunTask(ctx, owner, repo, issueNum, progress)
	if err != nil {
		h.send(chatID, fmt.Sprintf("❌ Erro: %s", err.Error()))
		return
	}
	h.send(chatID, fmt.Sprintf("✅ PR aberta: %s", result))
}

// parseTrigger extracts owner, repo, and issue number from a message.
// Falls back to defaultRepo when the message contains no explicit repo.
func parseTrigger(text, defaultRepo string) (owner, repo string, issueNum int, ok bool) {
	m := triggerRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", 0, false
	}
	num, err := strconv.Atoi(m[1])
	if err != nil {
		return "", "", 0, false
	}
	repoStr := m[2]
	if repoStr == "" {
		repoStr = defaultRepo
	}
	if repoStr == "" {
		return "", "", 0, false
	}
	parts := strings.SplitN(repoStr, "/", 2)
	if len(parts) != 2 {
		return "", "", 0, false
	}
	return parts[0], parts[1], num, true
}

func (h *TelegramHandler) send(chatID int64, text string) {
	_, _ = h.bot.Send(tgbotapi.NewMessage(chatID, text))
}
