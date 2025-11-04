package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/soaska/proxy/internal/speedtest"
	"github.com/soaska/proxy/internal/stats"
)

// Bot represents the Telegram bot
type Bot struct {
	api       *tgbotapi.BotAPI
	collector *stats.StatsCollector
	speedtest *speedtest.Service
	adminIDs  []int64
}

// NewBot creates a new Telegram bot
func NewBot(token string, adminIDs []int64, collector *stats.StatsCollector, st *speedtest.Service) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	_, err = api.Request(tgbotapi.DeleteWebhookConfig{
		DropPendingUpdates: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete webhook: %w", err)
	}

	bot := &Bot{
		api:       api,
		collector: collector,
		speedtest: st,
		adminIDs:  adminIDs,
	}

	// Set speedtest notification callback
	st.SetNotifyCallback(bot.onSpeedtestCompleted)

	log.Printf("[BOT] Authorized on account %s", api.Self.UserName)
	return bot, nil
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return fmt.Errorf("updates channel closed")
			}
			if update.Message == nil || update.Message.From == nil {
				continue
			}

			go b.handleMessage(update.Message)
		}
	}
}

// handleMessage processes incoming messages
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		if msg.IsCommand() && msg.Command() == "start" {
			reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf(
				"👋 Hello! Your Telegram ID is %d.\n\nThis bot is private and only admins can use it. Share your ID with an admin if you need access.",
				msg.From.ID,
			))
			b.api.Send(reply)
		} else {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Unauthorized. This bot is private.")
			b.api.Send(reply)
		}
		return
	}

	if !msg.IsCommand() {
		return
	}

	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "stats":
		b.handleStats(msg)
	case "speedtest":
		b.handleSpeedtest(msg)
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Unknown command. Use /start to see available commands.")
		b.api.Send(reply)
	}
}

// handleStart sends welcome message
func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := `
🤖 *Proxi Statistics Bot*

Available commands:

/stats - Server statistics
/speedtest - Run speed test

This bot provides detailed statistics for the SOCKS5 proxy server.
`
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleStats sends server statistics
func (b *Bot) handleStats(msg *tgbotapi.Message) {
	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get statistics")
		return
	}

	uptime := formatDuration(time.Duration(statsData.UptimeSeconds) * time.Second)

	text := fmt.Sprintf(`
📊 *Server Statistics*

⏱ Uptime: %s
🔗 Total Connections: %s
👥 Active Now: %d
📈 Total Traffic: %.2f GB

🌍 *Top Countries*
`, uptime, formatNumber(statsData.TotalConnections), statsData.ActiveConnections, statsData.TotalTrafficGB)

	for i, country := range statsData.Countries {
		if i >= 5 {
			break
		}
		text += fmt.Sprintf("%s %s: %.1f%% (%s)\n",
			getCountryFlag(country.Country),
			country.CountryName,
			country.Percentage,
			formatNumber(country.Connections))
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleSpeedtest runs a speedtest
func (b *Bot) handleSpeedtest(msg *tgbotapi.Message) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, "🔄 Running speed test... This may take a minute.")
	b.api.Send(reply)

	ctx := context.Background()
	result, err := b.speedtest.RunSpeedtest(ctx, fmt.Sprintf("bot:%s", msg.From.UserName), "")
	if err != nil {
		b.sendError(msg.Chat.ID, err.Error())
		return
	}

	text := fmt.Sprintf(`
✅ *Speed Test Complete*

⬇️ Download: *%.2f Mbps*
⬆️ Upload: *%.2f Mbps*
📡 Ping: *%.2f ms*

📍 Server: %s
🌐 Location: %s
🕐 Tested: %s
`, result.DownloadMbps, result.UploadMbps, result.PingMs,
		result.ServerName, result.ServerLocation, result.TestedAt.Format("15:04:05"))

	reply = tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// onSpeedtestCompleted sends notification when speedtest completes
func (b *Bot) onSpeedtestCompleted(result *speedtest.Result, triggeredBy, triggeredIP, triggeredCountry string) {
	for _, adminID := range b.adminIDs {
		text := fmt.Sprintf(`
🚀 *Speed Test Completed*

⬇️ Download: *%.2f Mbps*
⬆️ Upload: *%.2f Mbps*
📡 Ping: *%.2f ms*

📍 Server: %s (%s)
🕐 Time: %s

👤 Triggered by: %s
🌍 IP: %s (%s %s)
`, result.DownloadMbps, result.UploadMbps, result.PingMs,
			result.ServerName, result.ServerLocation,
			result.TestedAt.Format("15:04:05"),
			triggeredBy, triggeredIP,
			getCountryFlag(triggeredCountry), triggeredCountry)

		msg := tgbotapi.NewMessage(adminID, text)
		msg.ParseMode = "Markdown"
		b.api.Send(msg)
	}
}

// isAdmin checks if user is an admin
func (b *Bot) isAdmin(userID int64) bool {
	for _, id := range b.adminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// sendError sends error message
func (b *Bot) sendError(chatID int64, message string) {
	reply := tgbotapi.NewMessage(chatID, "❌ Error: "+message)
	b.api.Send(reply)
}

// Helper functions

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func formatNumber(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func getCountryFlag(code string) string {
	flags := map[string]string{
		"RU": "🇷🇺",
		"US": "🇺🇸",
		"DE": "🇩🇪",
		"GB": "🇬🇧",
		"FR": "🇫🇷",
		"NL": "🇳🇱",
		"CN": "🇨🇳",
		"JP": "🇯🇵",
		"KR": "🇰🇷",
		"IN": "🇮🇳",
		"BR": "🇧🇷",
		"CA": "🇨🇦",
		"AU": "🇦🇺",
		"IT": "🇮🇹",
		"ES": "🇪🇸",
		"PL": "🇵🇱",
		"UA": "🇺🇦",
		"TR": "🇹🇷",
		"SE": "🇸🇪",
		"NO": "🇳🇴",
	}
	if flag, ok := flags[code]; ok {
		return flag
	}
	return "🌍"
}
