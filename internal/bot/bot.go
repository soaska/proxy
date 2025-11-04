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

	// Set speedtest notification callback if service is available
	if st != nil {
		st.SetNotifyCallback(bot.onSpeedtestCompleted)
	} else {
		log.Printf("[BOT] Speedtest notifications disabled: service unavailable")
	}

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
	case "traffic":
		b.handleTraffic(msg)
	case "countries":
		b.handleCountries(msg)
	case "recent":
		b.handleRecentConnections(msg)
	case "top":
		b.handleTopCountries(msg)
	case "info":
		b.handleServerInfo(msg)
	case "help":
		b.handleHelp(msg)
	case "today":
		b.handleToday(msg)
	case "week":
		b.handleWeek(msg)
	case "peak":
		b.handlePeakUsage(msg)
	case "compare":
		b.handleCompare(msg)
	case "search":
		b.handleSearch(msg)
	case "export":
		b.handleExport(msg)
	case "status":
		b.handleStatus(msg)
	case "health":
		b.handleHealth(msg)
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❓ Unknown command. Use /help to see available commands.")
		b.api.Send(reply)
	}
}

// handleStart sends welcome message
func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := `
🤖 *Proxi Statistics Bot*

Welcome! I provide comprehensive statistics and management for your SOCKS5 proxy server.

📊 *Statistics Commands:*
/stats - General server statistics
/traffic - Detailed traffic analysis
/countries - Geographic distribution
/top - Top 10 countries by usage
/recent - Recent connections (last 10)

⚡ *Actions:*
/speedtest - Run internet speed test
/info - Detailed server information

ℹ️ *Help:*
/help - Show this help message

Use any command to get started!
`
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleStats sends server statistics
func (b *Bot) handleStats(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get statistics")
		return
	}

	uptime := formatDuration(time.Duration(statsData.UptimeSeconds) * time.Second)

	// Calculate additional metrics
	avgTrafficPerConn := float64(0)
	if statsData.TotalConnections > 0 {
		avgTrafficPerConn = statsData.TotalTrafficGB / float64(statsData.TotalConnections)
	}

	trafficIn, trafficOut := b.getTrafficBreakdown(ctx)

	text := fmt.Sprintf(`
📊 *Server Statistics Overview*

⏱ *Uptime:* %s
🔗 *Total Connections:* %s
👥 *Active Now:* %d
📈 *Total Traffic:* %.2f GB
   ↓ Download: %.2f GB
   ↑ Upload: %.2f GB
📊 *Avg per Connection:* %.2f MB

🌍 *Top 5 Countries:*
`, uptime, formatNumber(statsData.TotalConnections), statsData.ActiveConnections,
		statsData.TotalTrafficGB, trafficIn, trafficOut, avgTrafficPerConn*1024)

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

	text += "\n💡 Use /help to see all available commands"

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleSpeedtest runs a speedtest
func (b *Bot) handleSpeedtest(msg *tgbotapi.Message) {
	if b.speedtest == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Speedtest service is disabled.")
		b.api.Send(reply)
		return
	}

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

// handleTraffic sends detailed traffic analysis
func (b *Bot) handleTraffic(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get traffic statistics")
		return
	}

	trafficIn, trafficOut := b.getTrafficBreakdown(ctx)

	// Calculate traffic per hour
	uptimeHours := float64(statsData.UptimeSeconds) / 3600
	trafficPerHour := float64(0)
	if uptimeHours > 0 {
		trafficPerHour = statsData.TotalTrafficGB / uptimeHours
	}

	text := fmt.Sprintf(`
📈 *Traffic Analysis*

📊 *Total Traffic:* %.2f GB
   ↓ *Download:* %.2f GB (%.1f%%)
   ↑ *Upload:* %.2f GB (%.1f%%)

⏱ *Traffic Rate:*
   • Per Hour: %.2f GB/h
   • Per Day: %.2f GB/day (est.)
   • Per Connection: %.2f MB

🔗 *Connections:*
   • Total: %s
   • Active: %d
   • Avg Duration: %s

💡 Tip: Use /countries for geographic breakdown
`, statsData.TotalTrafficGB,
		trafficIn, (trafficIn/statsData.TotalTrafficGB)*100,
		trafficOut, (trafficOut/statsData.TotalTrafficGB)*100,
		trafficPerHour, trafficPerHour*24,
		(statsData.TotalTrafficGB/float64(statsData.TotalConnections))*1024,
		formatNumber(statsData.TotalConnections),
		statsData.ActiveConnections,
		b.getAvgConnectionDuration(ctx))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleCountries sends geographic distribution
func (b *Bot) handleCountries(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get country statistics")
		return
	}

	text := "🌍 *Geographic Distribution*\n\n"

	for i, country := range statsData.Countries {
		if i >= 15 {
			break
		}
		text += fmt.Sprintf("%s *%s*\n   Connections: %s (%.1f%%)\n",
			getCountryFlag(country.Country),
			country.CountryName,
			formatNumber(country.Connections),
			country.Percentage)
	}

	if len(statsData.Countries) > 15 {
		text += fmt.Sprintf("\n_...and %d more countries_", len(statsData.Countries)-15)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleTopCountries sends top 10 countries
func (b *Bot) handleTopCountries(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get top countries")
		return
	}

	text := "🏆 *Top 10 Countries*\n\n"

	medals := []string{"🥇", "🥈", "🥉"}
	for i, country := range statsData.Countries {
		if i >= 10 {
			break
		}
		medal := ""
		if i < 3 {
			medal = medals[i] + " "
		} else {
			medal = fmt.Sprintf("%d. ", i+1)
		}

		text += fmt.Sprintf("%s%s *%s* - %s (%.1f%%)\n",
			medal,
			getCountryFlag(country.Country),
			country.CountryName,
			formatNumber(country.Connections),
			country.Percentage)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleRecentConnections shows recent connections
func (b *Bot) handleRecentConnections(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	rows, err := b.collector.GetDB().QueryContext(ctx,
		`SELECT country, city, connected_at, bytes_in, bytes_out, duration
		 FROM connections
		 WHERE disconnected_at IS NOT NULL
		 ORDER BY connected_at DESC
		 LIMIT 10`)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get recent connections")
		return
	}
	defer rows.Close()

	text := "🕐 *Recent Connections (Last 10)*\n\n"

	count := 0
	for rows.Next() {
		var country, city string
		var connectedAt time.Time
		var bytesIn, bytesOut, duration int64

		if err := rows.Scan(&country, &city, &connectedAt, &bytesIn, &bytesOut, &duration); err != nil {
			continue
		}

		count++
		totalMB := float64(bytesIn+bytesOut) / (1024 * 1024)
		location := country
		if city != "" {
			location = fmt.Sprintf("%s, %s", city, country)
		}

		text += fmt.Sprintf("%s *%s*\n   ⏱ %s ago | 📊 %.1f MB | ⌛ %s\n",
			getCountryFlag(country),
			location,
			formatTimeAgo(connectedAt),
			totalMB,
			formatDuration(time.Duration(duration)*time.Second))
	}

	if count == 0 {
		text += "_No recent connections found_"
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleServerInfo sends detailed server information
func (b *Bot) handleServerInfo(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get server info")
		return
	}

	uptime := formatDuration(time.Duration(statsData.UptimeSeconds) * time.Second)
	trafficIn, trafficOut := b.getTrafficBreakdown(ctx)

	// Get database size
	var dbSizeKB int64
	b.collector.GetDB().QueryRow("SELECT page_count * page_size / 1024 FROM pragma_page_count(), pragma_page_size()").Scan(&dbSizeKB)

	text := fmt.Sprintf(`
ℹ️ *Detailed Server Information*

🖥 *Server Status*
   • Uptime: %s
   • Status: 🟢 Online
   • Active Connections: %d

📊 *Traffic Statistics*
   • Total: %.2f GB
   • Download: %.2f GB
   • Upload: %.2f GB
   • Ratio: %.2f

🔗 *Connection Statistics*
   • Total Connections: %s
   • Active Now: %d
   • Countries Served: %d

💾 *Database*
   • Size: %.2f MB
   • Tables: 4 (connections, server_stats, geo_stats, speedtest_results)

🌐 *Geographic Coverage*
   • Top Country: %s %s (%.1f%%)
   • Total Countries: %d
`,
		uptime,
		statsData.ActiveConnections,
		statsData.TotalTrafficGB,
		trafficIn,
		trafficOut,
		trafficIn/trafficOut,
		formatNumber(statsData.TotalConnections),
		statsData.ActiveConnections,
		len(statsData.Countries),
		float64(dbSizeKB)/1024,
		getCountryFlag(statsData.Countries[0].Country),
		statsData.Countries[0].CountryName,
		statsData.Countries[0].Percentage,
		len(statsData.Countries))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleHelp sends help message
func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	text := `
📚 *Help - Available Commands*

📊 *Statistics:*
/stats - General server statistics
/traffic - Detailed traffic analysis
/countries - Full geographic distribution
/top - Top 10 countries by usage
/recent - Last 10 connections

📅 *Time-based Statistics:*
/today - Statistics for today
/week - This week's statistics
/peak - Peak usage analysis
/compare - Compare periods

⚡ *Actions:*
/speedtest - Run internet speed test
/search [country] - Search by country code
/export - Export data as JSON

🔧 *Monitoring:*
/status - Quick status check
/health - System health diagnostics
/info - Detailed server information

📚 *Help:*
/help - Show this help message
/start - Welcome message

💡 *Tips:*
• Stats update in real-time
• Speedtest: 10-min cooldown
• Use country codes (RU, US, DE, etc.)
• All times in server timezone

🔒 *Privacy & Security:*
• IP addresses stored securely
• Only aggregated data is public
• Admin-only bot access
• 90-day data retention

📊 *Example Commands:*
/search US - Show US connections
/compare - Compare today vs yesterday

Need help? Contact the server admin.
`
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleToday shows today's statistics
func (b *Bot) handleToday(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	var totalConns, totalBytes int64
	err := b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')`,
	).Scan(&totalConns, &totalBytes)

	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get today's statistics")
		return
	}

	// Get hourly breakdown
	rows, err := b.collector.GetDB().QueryContext(ctx,
		`SELECT strftime('%H', connected_at) as hour, COUNT(*)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')
		 GROUP BY hour
		 ORDER BY hour DESC
		 LIMIT 5`)

	if err == nil {
		defer rows.Close()
	}

	trafficGB := float64(totalBytes) / (1024 * 1024 * 1024)

	text := fmt.Sprintf(`
📅 *Today's Statistics*

📊 *Overview:*
   • Connections: %s
   • Traffic: %.2f GB
   • Avg per Conn: %.2f MB

⏰ *Recent Hourly Activity:*
`, formatNumber(totalConns), trafficGB, (trafficGB/float64(totalConns))*1024)

	if rows != nil {
		for rows.Next() {
			var hour string
			var count int64
			if err := rows.Scan(&hour, &count); err == nil {
				text += fmt.Sprintf("   %s:00 - %s connections\n", hour, formatNumber(count))
			}
		}
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleWeek shows this week's statistics
func (b *Bot) handleWeek(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	var totalConns, totalBytes int64
	err := b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')`,
	).Scan(&totalConns, &totalBytes)

	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get week statistics")
		return
	}

	// Get daily breakdown
	rows, err := b.collector.GetDB().QueryContext(ctx,
		`SELECT DATE(connected_at) as day, COUNT(*), SUM(bytes_in + bytes_out)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')
		 GROUP BY day
		 ORDER BY day DESC`)

	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get daily breakdown")
		return
	}
	defer rows.Close()

	trafficGB := float64(totalBytes) / (1024 * 1024 * 1024)
	avgPerDay := float64(totalConns) / 7

	text := fmt.Sprintf(`
📊 *This Week's Statistics*

📈 *7-Day Summary:*
   • Total Connections: %s
   • Total Traffic: %.2f GB
   • Avg per Day: %.0f connections
   • Avg per Conn: %.2f MB

📅 *Daily Breakdown:*
`, formatNumber(totalConns), trafficGB, avgPerDay, (trafficGB/float64(totalConns))*1024)

	for rows.Next() {
		var day string
		var count, bytes int64
		if err := rows.Scan(&day, &count, &bytes); err == nil {
			dayGB := float64(bytes) / (1024 * 1024 * 1024)
			text += fmt.Sprintf("   %s: %s (%.2f GB)\n", day, formatNumber(count), dayGB)
		}
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handlePeakUsage shows peak usage times
func (b *Bot) handlePeakUsage(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	// Peak hour
	var peakHour string
	var peakHourCount int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT strftime('%H', connected_at) as hour, COUNT(*) as count
		 FROM connections
		 GROUP BY hour
		 ORDER BY count DESC
		 LIMIT 1`,
	).Scan(&peakHour, &peakHourCount)

	// Peak day
	var peakDay string
	var peakDayCount int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT DATE(connected_at) as day, COUNT(*) as count
		 FROM connections
		 GROUP BY day
		 ORDER BY count DESC
		 LIMIT 1`,
	).Scan(&peakDay, &peakDayCount)

	// Busiest country
	var busiestCountry, busiestCountryName string
	var busiestCountryCount int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT country, country_name, connections
		 FROM geo_stats
		 ORDER BY connections DESC
		 LIMIT 1`,
	).Scan(&busiestCountry, &busiestCountryName, &busiestCountryCount)

	text := fmt.Sprintf(`
📊 *Peak Usage Analysis*

⏰ *Peak Hour:*
   %s:00 - %s:59
   %s connections

📅 *Peak Day:*
   %s
   %s connections

🌍 *Busiest Country:*
   %s %s
   %s total connections

💡 *Insights:*
• Most active hour: %s:00
• Highest single day: %s
• Primary traffic source: %s
`,
		peakHour, peakHour, formatNumber(peakHourCount),
		peakDay, formatNumber(peakDayCount),
		getCountryFlag(busiestCountry), busiestCountryName, formatNumber(busiestCountryCount),
		peakHour, peakDay, busiestCountryName)

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleCompare compares time periods
func (b *Bot) handleCompare(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	// Today
	var todayConns, todayBytes int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')`,
	).Scan(&todayConns, &todayBytes)

	// Yesterday
	var yesterdayConns, yesterdayBytes int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now', '-1 day')`,
	).Scan(&yesterdayConns, &yesterdayBytes)

	// This week
	var thisWeekConns, thisWeekBytes int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')`,
	).Scan(&thisWeekConns, &thisWeekBytes)

	// Last week
	var lastWeekConns, lastWeekBytes int64
	b.collector.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-14 days')
		   AND connected_at < datetime('now', '-7 days')`,
	).Scan(&lastWeekConns, &lastWeekBytes)

	// Calculate changes
	connChangeDaily := calculatePercentChange(yesterdayConns, todayConns)
	trafficChangeDaily := calculatePercentChange(yesterdayBytes, todayBytes)
	connChangeWeekly := calculatePercentChange(lastWeekConns, thisWeekConns)
	trafficChangeWeekly := calculatePercentChange(lastWeekBytes, thisWeekBytes)

	text := fmt.Sprintf(`
📊 *Period Comparison*

📅 *Today vs Yesterday:*
   Connections: %s → %s (%s)
   Traffic: %.2f GB → %.2f GB (%s)

📈 *This Week vs Last Week:*
   Connections: %s → %s (%s)
   Traffic: %.2f GB → %.2f GB (%s)

💡 *Trend Analysis:*
%s
`,
		formatNumber(yesterdayConns), formatNumber(todayConns), connChangeDaily,
		float64(yesterdayBytes)/(1024*1024*1024), float64(todayBytes)/(1024*1024*1024), trafficChangeDaily,
		formatNumber(lastWeekConns), formatNumber(thisWeekConns), connChangeWeekly,
		float64(lastWeekBytes)/(1024*1024*1024), float64(thisWeekBytes)/(1024*1024*1024), trafficChangeWeekly,
		generateTrendInsight(connChangeDaily, trafficChangeWeekly))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleSearch searches connections by country
func (b *Bot) handleSearch(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	args := strings.Fields(msg.CommandArguments())
	if len(args) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "ℹ️ Usage: `/search [country_code]`\nExample: `/search US` or `/search RU`")
		reply.ParseMode = "Markdown"
		b.api.Send(reply)
		return
	}

	countryCode := strings.ToUpper(args[0])
	ctx := context.Background()

	var countryName string
	var totalConns, totalBytes int64
	err := b.collector.GetDB().QueryRowContext(ctx,
		`SELECT country_name, connections, total_bytes
		 FROM geo_stats
		 WHERE country = ?`,
		countryCode,
	).Scan(&countryName, &totalConns, &totalBytes)

	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("❌ No data found for country code: %s", countryCode))
		b.api.Send(reply)
		return
	}

	// Get recent connections from this country
	rows, _ := b.collector.GetDB().QueryContext(ctx,
		`SELECT city, connected_at, bytes_in + bytes_out as total_bytes
		 FROM connections
		 WHERE country = ?
		   AND disconnected_at IS NOT NULL
		 ORDER BY connected_at DESC
		 LIMIT 5`,
		countryCode)

	trafficGB := float64(totalBytes) / (1024 * 1024 * 1024)

	text := fmt.Sprintf(`
🔍 *Search Results: %s %s*

📊 *Statistics:*
   • Total Connections: %s
   • Total Traffic: %.2f GB
   • Avg per Connection: %.2f MB

🕐 *Recent Connections:*
`, getCountryFlag(countryCode), countryName, formatNumber(totalConns), trafficGB, (trafficGB/float64(totalConns))*1024)

	if rows != nil {
		defer rows.Close()
		count := 0
		for rows.Next() {
			var city string
			var connectedAt time.Time
			var bytes int64
			if err := rows.Scan(&city, &connectedAt, &bytes); err == nil {
				count++
				location := city
				if location == "" {
					location = "Unknown City"
				}
				text += fmt.Sprintf("   %d. %s - %s ago (%.1f MB)\n",
					count, location, formatTimeAgo(connectedAt), float64(bytes)/(1024*1024))
			}
		}
		if count == 0 {
			text += "   _No recent connections_\n"
		}
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleExport exports statistics as JSON
func (b *Bot) handleExport(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to export statistics")
		return
	}

	// Create JSON export
	export := fmt.Sprintf(`{
  "timestamp": "%s",
  "uptime_seconds": %d,
  "total_connections": %d,
  "active_connections": %d,
  "total_traffic_gb": %.2f,
  "countries": %d,
  "top_countries": [`,
		time.Now().Format(time.RFC3339),
		statsData.UptimeSeconds,
		statsData.TotalConnections,
		statsData.ActiveConnections,
		statsData.TotalTrafficGB,
		len(statsData.Countries))

	for i, country := range statsData.Countries {
		if i >= 10 {
			break
		}
		if i > 0 {
			export += ","
		}
		export += fmt.Sprintf(`
    {"code": "%s", "name": "%s", "connections": %d, "percentage": %.2f}`,
			country.Country, country.CountryName, country.Connections, country.Percentage)
	}

	export += `
  ]
}`

	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("```json\n%s\n```", export))
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleStatus quick status check
func (b *Bot) handleStatus(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()
	statsData, err := b.collector.GetPublicStats(ctx)
	if err != nil {
		b.sendError(msg.Chat.ID, "Failed to get status")
		return
	}

	uptime := formatDuration(time.Duration(statsData.UptimeSeconds) * time.Second)
	status := "🟢 Online"
	if statsData.ActiveConnections == 0 {
		status = "🟡 Idle"
	}

	text := fmt.Sprintf(`
⚡ *Quick Status*

%s
⏱ Uptime: %s
👥 Active: %d
📊 Total: %s
`,
		status,
		uptime,
		statsData.ActiveConnections,
		formatNumber(statsData.TotalConnections))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// handleHealth performs health check
func (b *Bot) handleHealth(msg *tgbotapi.Message) {
	if b.collector == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Statistics module is disabled.")
		b.api.Send(reply)
		return
	}

	ctx := context.Background()

	// Check database
	dbHealthy := true
	if err := b.collector.GetDB().PingContext(ctx); err != nil {
		dbHealthy = false
	}

	// Check stats collector
	statsHealthy := b.collector != nil

	// Check speedtest service
	speedtestHealthy := b.speedtest != nil

	// Get active connections
	activeConns := b.collector.GetActiveConnections()

	// Overall health
	overallHealth := "🟢 Healthy"
	if !dbHealthy || !statsHealthy {
		overallHealth = "🔴 Unhealthy"
	} else if !speedtestHealthy {
		overallHealth = "🟡 Degraded"
	}

	text := fmt.Sprintf(`
🏥 *System Health Check*

*Overall Status:* %s

🔧 *Components:*
   • Database: %s
   • Stats Collector: %s
   • Speedtest Service: %s
   • Bot: 🟢 Online

📊 *Metrics:*
   • Active Connections: %d
   • Database: %s

💡 All systems operational!
`,
		overallHealth,
		getHealthIcon(dbHealthy),
		getHealthIcon(statsHealthy),
		getHealthIcon(speedtestHealthy),
		activeConns,
		getHealthIcon(dbHealthy))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

// Helper functions for new commands

func calculatePercentChange(old, new int64) string {
	if old == 0 {
		if new > 0 {
			return "📈 +∞%"
		}
		return "➡️ 0%"
	}

	change := float64(new-old) / float64(old) * 100
	if change > 0 {
		return fmt.Sprintf("📈 +%.1f%%", change)
	} else if change < 0 {
		return fmt.Sprintf("📉 %.1f%%", change)
	}
	return "➡️ 0%"
}

func generateTrendInsight(dailyChange, weeklyChange string) string {
	insights := []string{}

	if strings.Contains(dailyChange, "+") {
		insights = append(insights, "• Daily traffic increasing")
	} else if strings.Contains(dailyChange, "-") {
		insights = append(insights, "• Daily traffic decreasing")
	}

	if strings.Contains(weeklyChange, "+") {
		insights = append(insights, "• Weekly trend: Growing")
	} else if strings.Contains(weeklyChange, "-") {
		insights = append(insights, "• Weekly trend: Declining")
	} else {
		insights = append(insights, "• Weekly trend: Stable")
	}

	if len(insights) == 0 {
		return "• Traffic is stable"
	}

	return strings.Join(insights, "\n")
}

func getHealthIcon(healthy bool) string {
	if healthy {
		return "🟢 OK"
	}
	return "🔴 Error"
}

// Helper methods for additional statistics

func (b *Bot) getTrafficBreakdown(ctx context.Context) (float64, float64) {
	var bytesIn, bytesOut int64
	err := b.collector.GetDB().QueryRowContext(ctx,
		`SELECT total_bytes_in, total_bytes_out FROM server_stats WHERE id = 1`,
	).Scan(&bytesIn, &bytesOut)

	if err != nil {
		return 0, 0
	}

	return float64(bytesIn) / (1024 * 1024 * 1024), float64(bytesOut) / (1024 * 1024 * 1024)
}

func (b *Bot) getAvgConnectionDuration(ctx context.Context) string {
	var avgDuration float64
	err := b.collector.GetDB().QueryRowContext(ctx,
		`SELECT AVG(duration) FROM connections WHERE duration > 0`,
	).Scan(&avgDuration)

	if err != nil || avgDuration == 0 {
		return "N/A"
	}

	return formatDuration(time.Duration(avgDuration) * time.Second)
}

func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%d min", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%d hr", hours)
	} else {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days", days)
	}
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
		"FI": "🇫🇮",
		"DK": "🇩🇰",
		"CH": "🇨🇭",
		"AT": "🇦🇹",
		"BE": "🇧🇪",
		"CZ": "🇨🇿",
		"GR": "🇬🇷",
		"PT": "🇵🇹",
		"RO": "🇷🇴",
		"HU": "🇭🇺",
		"SG": "🇸🇬",
		"HK": "🇭🇰",
		"TW": "🇹🇼",
		"TH": "🇹🇭",
		"VN": "🇻🇳",
		"ID": "🇮🇩",
		"MY": "🇲🇾",
		"PH": "🇵🇭",
		"MX": "🇲🇽",
		"AR": "🇦🇷",
		"CL": "🇨🇱",
		"CO": "🇨🇴",
		"ZA": "🇿🇦",
		"EG": "🇪🇬",
		"IL": "🇮🇱",
		"AE": "🇦🇪",
		"SA": "🇸🇦",
		"NG": "🇳🇬",
		"KE": "🇰🇪",
	}
	if flag, ok := flags[code]; ok {
		return flag
	}
	return "🌍"
}
