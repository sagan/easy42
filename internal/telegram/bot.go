package telegram

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"easy42/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ClusterProvider defines the interface required by the Telegram bot to fetch easy42 cluster information
type ClusterProvider interface {
	GetFleetLiveStatus() (config.FleetMetricsSummary, []config.NodeLiveStatus, error)
	GetNetworkState() *config.NetworkState
}

// BotInfo holds runtime information about the connected Telegram bot
type BotInfo struct {
	Username      string `json:"username"`
	WebhookStatus string `json:"webhook_status"`
	WebhookError  string `json:"webhook_error,omitempty"`
}

// BotService manages the Telegram bot lifecycle, webhook updates, and node status alerts
type BotService struct {
	mu                     sync.RWMutex
	provider               ClusterProvider
	settings               config.IntegrationsSettings
	bot                    *tgbotapi.BotAPI
	botUsername            string
	webhookStatus          string
	webhookError           string
	stopCh                 chan struct{}
	monitorRunning         bool
	prevNodeOnline         map[string]bool
	monitorInitDone        bool
	onAlert                func(text string)
	onSecretTokenGenerated func(token string)
}

// SetOnSecretTokenGenerated registers a callback to save an auto-generated webhook secret token
func (s *BotService) SetOnSecretTokenGenerated(fn func(token string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSecretTokenGenerated = fn
}

// NewBotService creates a new BotService instance
func NewBotService(provider ClusterProvider) *BotService {
	return &BotService{
		provider:       provider,
		prevNodeOnline: make(map[string]bool),
	}
}

// Start initializes the Telegram bot and begins monitoring based on the given settings
func (s *BotService) Start(settings config.IntegrationsSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = settings
	if !settings.TelegramEnabled || strings.TrimSpace(settings.TelegramBotToken) == "" {
		s.stopUnlocked()
		s.webhookStatus = "disabled"
		s.webhookError = ""
		return nil
	}

	return s.initBotUnlocked()
}

// Reload updates settings and reconfigures the bot
func (s *BotService) Reload(settings config.IntegrationsSettings) error {
	return s.Start(settings)
}

// Stop terminates background monitoring and cleans up bot resources
func (s *BotService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopUnlocked()
}

func (s *BotService) stopUnlocked() {
	if s.monitorRunning && s.stopCh != nil {
		close(s.stopCh)
		s.monitorRunning = false
		s.stopCh = nil
	}
	s.bot = nil
	s.botUsername = ""
}

// initBotUnlocked creates the bot instance, configures webhooks, commands, menu buttons, and starts monitoring
func (s *BotService) initBotUnlocked() error {
	s.stopUnlocked()

	token := strings.TrimSpace(s.settings.TelegramBotToken)
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		s.webhookStatus = "error"
		s.webhookError = fmt.Sprintf("invalid bot token: %v", err)
		return fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	s.bot = bot
	s.botUsername = bot.Self.UserName
	s.webhookStatus = "initializing"
	s.webhookError = ""

	// Register webhook if server URL is provided
	serverURL := strings.TrimSpace(s.settings.ServerURL)
	if serverURL != "" {
		webhookURL := fmt.Sprintf("%s/api/telegram/webhook", strings.TrimRight(serverURL, "/"))
		secretToken := strings.TrimSpace(s.settings.TelegramSecretToken)
		if secretToken == "" {
			secretToken = config.GenerateTelegramSecretToken()
			s.settings.TelegramSecretToken = secretToken
			if s.onSecretTokenGenerated != nil {
				s.onSecretTokenGenerated(secretToken)
			}
		}
		params := tgbotapi.Params{
			"url":          webhookURL,
			"secret_token": secretToken,
		}
		resp, err := bot.MakeRequest("setWebhook", params)
		if err != nil || (resp != nil && !resp.Ok) {
			errDesc := ""
			if err != nil {
				errDesc = err.Error()
			} else if resp != nil {
				errDesc = resp.Description
			}
			s.webhookStatus = "webhook_error"
			s.webhookError = fmt.Sprintf("failed to set webhook: %s", errDesc)
			log.Printf("[telegram] Warning: failed to register webhook with URL %s: %s", webhookURL, errDesc)
		} else {
			s.webhookStatus = "registered"
		}
	} else {
		s.webhookStatus = "no_server_url"
	}

	// Register bot commands list (/status, /nodes, /start, /help)
	cmdConfig := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "status", Description: "Query easy42 cluster overview status"},
		tgbotapi.BotCommand{Command: "nodes", Description: "Detailed node statuses and metrics"},
		tgbotapi.BotCommand{Command: "start", Description: "Welcome & usage instructions"},
		tgbotapi.BotCommand{Command: "help", Description: "Show available commands"},
	)
	if _, err := bot.Request(cmdConfig); err != nil {
		log.Printf("[telegram] Warning: failed to register bot commands: %v", err)
	}

	// Register ChatMenuButton to show commands menu in Telegram clients
	menuParams := tgbotapi.Params{
		"menu_button": `{"type":"commands"}`,
	}
	if _, err := bot.MakeRequest("setChatMenuButton", menuParams); err != nil {
		log.Printf("[telegram] Note: setChatMenuButton result: %v", err)
	}

	// Start background node health alert monitor
	s.stopCh = make(chan struct{})
	s.monitorRunning = true
	s.monitorInitDone = false
	s.prevNodeOnline = make(map[string]bool)

	go s.monitorLoop(s.stopCh)

	return nil
}

// GetBotInfo returns information about the current Telegram bot connection
func (s *BotService) GetBotInfo() BotInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return BotInfo{
		Username:      s.botUsername,
		WebhookStatus: s.webhookStatus,
		WebhookError:  s.webhookError,
	}
}

// HandleWebhookUpdate processes an incoming Telegram update received via HTTP webhook
func (s *BotService) HandleWebhookUpdate(update *tgbotapi.Update) error {
	s.mu.RLock()
	bot := s.bot
	adminID := s.settings.TelegramAdminID
	serverURL := s.settings.ServerURL
	s.mu.RUnlock()

	if bot == nil {
		return fmt.Errorf("telegram bot is not initialized")
	}

	if update.Message == nil {
		return nil
	}

	msg := update.Message
	senderID := msg.From.ID

	// Implicit authentication check: only admin tg userid is allowed to interact
	if adminID == 0 || senderID != adminID {
		denied := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("⛔ <b>Access Denied</b>\n\nYour Telegram User ID (<code>%d</code>) is not authorized to interact with this easy42 server.", senderID))
		denied.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(denied)
		return nil
	}

	text := strings.TrimSpace(msg.Text)
	command := strings.ToLower(text)
	if strings.HasPrefix(command, "/") {
		// Strip bot username if invoked as /command@botname
		parts := strings.SplitN(command, "@", 2)
		command = parts[0]
	}

	switch command {
	case "/start":
		return s.handleStartCommand(msg.Chat.ID, serverURL)
	case "/help":
		return s.handleHelpCommand(msg.Chat.ID, serverURL)
	case "/status":
		return s.handleStatusCommand(msg.Chat.ID, serverURL)
	case "/nodes":
		return s.handleNodesCommand(msg.Chat.ID, serverURL)
	default:
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Unrecognized command. Type /status to check cluster status, or /help to see available commands.")
		_, err := bot.Send(reply)
		return err
	}
}

func (s *BotService) handleStartCommand(chatID int64, serverURL string) error {
	webLink := ""
	if serverURL != "" {
		webLink = fmt.Sprintf("\n\n🌐 Open easy42 Dashboard: %s", serverURL)
	}

	text := fmt.Sprintf(`👋 <b>Welcome to easy42 Telegram Bot!</b>

I am your easy42 overlay network monitoring and notification assistant.

<b>Available Commands:</b>
• /status - Query current easy42 cluster overview and live metrics
• /nodes - Detailed node list, agent statuses, and resource usage
• /help - Show available commands and instructions%s`, webLink)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err := s.bot.Send(msg)
	return err
}

func (s *BotService) handleHelpCommand(chatID int64, serverURL string) error {
	webLink := ""
	if serverURL != "" {
		webLink = fmt.Sprintf("\n\n🌐 Open easy42 dashboard: %s", serverURL)
	}

	text := fmt.Sprintf(`📖 <b>easy42 Telegram Bot Commands</b>
━━━━━━━━━━━━━━━━━━━━
• <b>/status</b>: Query high-level cluster health, online/offline node counts, and fleet traffic rates.
• <b>/nodes</b>: Inspect each node's connection state, uptime, IP, and hardware resource metrics.
• <b>/help</b>: Display this help message.

⚡ <b>Automated Notifications:</b>
When nodes go offline or reconnect back online, you will receive instant alert notifications here.%s`, webLink)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err := s.bot.Send(msg)
	return err
}

func (s *BotService) handleStatusCommand(chatID int64, serverURL string) error {
	if s.provider == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Cluster provider not available.")
		_, err := s.bot.Send(msg)
		return err
	}

	summary, nodes, err := s.provider.GetFleetLiveStatus()
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Error querying fleet status: %v", err))
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	webLink := ""
	if serverURL != "" {
		webLink = fmt.Sprintf("\n\n🖥️ Open easy42 Dashboard: %s", serverURL)
	}

	rxFormatted := formatBytesRate(summary.TotalRxRate)
	txFormatted := formatBytesRate(summary.TotalTxRate)

	text := fmt.Sprintf(`🌐 <b>easy42 Cluster Status</b>
━━━━━━━━━━━━━━━━━━━━
📊 <b>Fleet Nodes (%d total):</b>
• 🟢 Online: <b>%d</b>
• 🔴 Offline: <b>%d</b>
• ⚪ No Agent: <b>%d</b>

⚡ <b>Traffic & Fleet Performance:</b>
• Aggregate Rx: <b>%s</b>
• Aggregate Tx: <b>%s</b>
• Average Fleet CPU: <b>%.1f%%</b>
• Average Fleet Memory: <b>%.1f%%</b>

<i>Total managed nodes evaluated: %d</i>%s`,
		summary.TotalNodes,
		summary.OnlineNodes,
		summary.OfflineNodes,
		summary.NoAgentNodes,
		rxFormatted,
		txFormatted,
		summary.AverageCPU,
		summary.AverageMemPerc,
		len(nodes),
		webLink,
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err = s.bot.Send(msg)
	return err
}

func (s *BotService) handleNodesCommand(chatID int64, serverURL string) error {
	if s.provider == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Cluster provider not available.")
		_, err := s.bot.Send(msg)
		return err
	}

	_, nodes, err := s.provider.GetFleetLiveStatus()
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Error querying nodes: %v", err))
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	if len(nodes) == 0 {
		msg := tgbotapi.NewMessage(chatID, "ℹ️ No managed nodes in the cluster.")
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	var sb strings.Builder
	sb.WriteString("🖥️ <b>easy42 Cluster Nodes:</b>\n━━━━━━━━━━━━━━━━━━━━\n")

	for _, n := range nodes {
		var icon string
		var statusText string

		if n.Connected {
			icon = "🟢"
			cpuStr := "N/A"
			memStr := "N/A"
			if n.Metrics != nil {
				cpuStr = fmt.Sprintf("%.0f%%", n.Metrics.CPUPercent)
				if n.Metrics.MemoryTotalBytes > 0 {
					memPerc := float64(n.Metrics.MemoryUsedBytes) / float64(n.Metrics.MemoryTotalBytes) * 100.0
					memStr = fmt.Sprintf("%.0f%%", memPerc)
				}
			}
			uptimeStr := formatUptime(n.UptimeSeconds)
			statusText = fmt.Sprintf("Online | CPU: %s | Mem: %s | Up: %s", cpuStr, memStr, uptimeStr)
		} else if n.AgentInstalled {
			icon = "🔴"
			lastSeenStr := "never"
			if !n.LastSeen.IsZero() {
				lastSeenStr = time.Since(n.LastSeen).Truncate(time.Second).String() + " ago"
			}
			statusText = fmt.Sprintf("Offline (Last seen: %s)", lastSeenStr)
		} else {
			icon = "⚪"
			statusText = "No agent installed"
		}

		sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n   ↳ <i>%s</i>\n", icon, n.Name, statusText))
	}

	if serverURL != "" {
		sb.WriteString(fmt.Sprintf("\n🌐 <a href=\"%s/#/nodes\">View Nodes in Dashboard</a>", strings.TrimRight(serverURL, "/")))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err = s.bot.Send(msg)
	return err
}

// monitorLoop periodically checks node statuses and fires alerts when nodes transition online/offline
func (s *BotService) monitorLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.checkNodeStatusTransitions()
		}
	}
}

func (s *BotService) checkNodeStatusTransitions() {
	s.mu.RLock()
	bot := s.bot
	adminID := s.settings.TelegramAdminID
	serverURL := s.settings.ServerURL
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return
	}

	_, nodes, err := provider.GetFleetLiveStatus()
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.monitorInitDone {
		// Initial baseline recording - do not send alerts on first run
		for _, n := range nodes {
			s.prevNodeOnline[n.Name] = n.Connected
		}
		s.monitorInitDone = true
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, n := range nodes {
		wasOnline, existed := s.prevNodeOnline[n.Name]
		isOnline := n.Connected

		if existed {
			if wasOnline && !isOnline {
				// Node went OFFLINE
				s.prevNodeOnline[n.Name] = false

				dashLink := ""
				if serverURL != "" {
					dashLink = fmt.Sprintf("\n\nOpen easy42 Dashboard: %s", strings.TrimRight(serverURL, "/"))
				}

				alert := fmt.Sprintf(`🔴 <b>easy42 Alert: Node Offline</b>

Node: <b>%s</b>
Hostname: <code>%s</code>
Status: <b>Disconnected / Unreachable</b>
Time: <code>%s UTC</code>%s`, n.Name, n.Hostname, now, dashLink)

				if s.onAlert != nil {
					s.onAlert(alert)
				}
				if bot != nil && adminID != 0 {
					go s.sendNotificationDirect(bot, adminID, alert)
				}
			} else if !wasOnline && isOnline {
				// Node went BACK ONLINE
				s.prevNodeOnline[n.Name] = true

				dashLink := ""
				if serverURL != "" {
					dashLink = fmt.Sprintf("\n\nOpen easy42 Dashboard: %s", strings.TrimRight(serverURL, "/"))
				}

				agentVer := n.AgentVersion
				if agentVer == "" {
					agentVer = "active"
				}

				alert := fmt.Sprintf(`🟢 <b>easy42 Alert: Node Online</b>

Node: <b>%s</b>
Hostname: <code>%s</code>
Status: <b>Connected & Active</b> (%s)
Time: <code>%s UTC</code>%s`, n.Name, n.Hostname, agentVer, now, dashLink)

				if s.onAlert != nil {
					s.onAlert(alert)
				}
				if bot != nil && adminID != 0 {
					go s.sendNotificationDirect(bot, adminID, alert)
				}
			}
		} else {
			s.prevNodeOnline[n.Name] = isOnline
		}
	}
}

func (s *BotService) sendNotificationDirect(bot *tgbotapi.BotAPI, chatID int64, htmlText string) {
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	if _, err := bot.Send(msg); err != nil {
		log.Printf("[telegram] Failed to send notification to %d: %v", chatID, err)
	}
}

// SendAlert sends a notification message to the configured Telegram admin user
func (s *BotService) SendAlert(htmlText string) error {
	s.mu.RLock()
	bot := s.bot
	adminID := s.settings.TelegramAdminID
	s.mu.RUnlock()

	if bot == nil {
		return fmt.Errorf("telegram bot is not running")
	}
	if adminID == 0 {
		return fmt.Errorf("no admin user ID configured")
	}

	msg := tgbotapi.NewMessage(adminID, htmlText)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	return err
}

// TestConnection verifies credentials with Telegram API and sends a test message to the admin user
func (s *BotService) TestConnection(token string, adminID int64, serverURL string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("telegram bot token is required")
	}
	if adminID == 0 {
		return "", fmt.Errorf("admin telegram user ID is required")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Telegram with provided token: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	serverInfo := "Not configured"
	if serverURL != "" {
		serverInfo = serverURL
	}

	testText := fmt.Sprintf(`✅ <b>easy42 Telegram Bot Integration Test</b>

Your easy42 Telegram bot is properly configured and successfully communicating!

🤖 <b>Bot:</b> @%s
👤 <b>Admin ID:</b> <code>%d</code>
🌐 <b>Server URL:</b> %s
⏰ <b>Time:</b> <code>%s UTC</code>

You will receive real-time notifications here when nodes go online or offline, and you can query cluster status with <b>/status</b>.`,
		bot.Self.UserName,
		adminID,
		serverInfo,
		now,
	)

	msg := tgbotapi.NewMessage(adminID, testText)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	if _, err := bot.Send(msg); err != nil {
		return bot.Self.UserName, fmt.Errorf("bot authorization succeeded (@%s), but failed to send test message to user %d: %w (ensure you have opened the bot in Telegram and clicked 'Start')", bot.Self.UserName, adminID, err)
	}

	return bot.Self.UserName, nil
}

func formatBytesRate(b float64) string {
	if b <= 0 {
		return "0 B/s"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%.0f B/s", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", b/float64(div), "KMGTPE"[exp])
}

func formatUptime(seconds uint64) string {
	if seconds == 0 {
		return "just started"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
