package telegram

import (
	"context"
	"fmt"
	"html"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy42/internal/config"
	"easy42/internal/lookingglass"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ClusterProvider defines the interface required by the Telegram bot to fetch easy42 cluster information
type ClusterProvider interface {
	GetNodes() []config.Node
	GetLinks() []config.Link
	GetFleetLiveStatus() (config.FleetMetricsSummary, []config.NodeLiveStatus, error)
	GetNetworkState() *config.NetworkState
	RunLookingGlass(ctx context.Context, req lookingglass.RunRequest) (lookingglass.RunResponse, error)
}

// BotInfo holds runtime information about the connected Telegram bot
type BotInfo struct {
	Username      string `json:"username"`
	WebhookStatus string `json:"webhook_status"`
	WebhookError  string `json:"webhook_error,omitempty"`
}

// nodeDebounceState tracks the debounced online/offline transition state of a node
type nodeDebounceState struct {
	notifiedOnline bool
	pendingOnline  bool
	pendingCount   int
	pendingSince   time.Time
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
	nodeStates             map[string]*nodeDebounceState
	monitorInitDone        bool
	monitorStartedAt       time.Time
	startupGracePeriod     time.Duration
	debounceCount          int
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
		provider:           provider,
		prevNodeOnline:     make(map[string]bool),
		nodeStates:         make(map[string]*nodeDebounceState),
		startupGracePeriod: 90 * time.Second,
		debounceCount:      2,
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

	// Register bot commands list (/status, /nodes, /node, /ping, /tcp_ping, /start, /help)
	cmdConfig := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "status", Description: "Query easy42 cluster overview status"},
		tgbotapi.BotCommand{Command: "nodes", Description: "Detailed node statuses and metrics"},
		tgbotapi.BotCommand{Command: "node", Description: "Detailed info and state of a node"},
		tgbotapi.BotCommand{Command: "ping", Description: "Looking Glass ICMP ping test"},
		tgbotapi.BotCommand{Command: "tcp_ping", Description: "Looking Glass TCP ping test"},
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
	s.monitorStartedAt = time.Now()
	s.prevNodeOnline = make(map[string]bool)
	s.nodeStates = make(map[string]*nodeDebounceState)
	if s.startupGracePeriod <= 0 {
		s.startupGracePeriod = 90 * time.Second
	}
	if s.debounceCount <= 0 {
		s.debounceCount = 2
	}

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

	var chatID int64
	var text string
	var senderID int64

	if update.Message != nil {
		chatID = update.Message.Chat.ID
		text = update.Message.Text
		if update.Message.From != nil {
			senderID = update.Message.From.ID
		}
	} else if update.CallbackQuery != nil {
		if update.CallbackQuery.Message != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		}
		if update.CallbackQuery.From != nil {
			senderID = update.CallbackQuery.From.ID
			if chatID == 0 {
				chatID = senderID
			}
		}
		text = update.CallbackQuery.Data

		// Acknowledge callback query to stop client spinner
		callbackResp := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
		_, _ = bot.Request(callbackResp)
	} else {
		return nil
	}

	// Implicit authentication check: only admin tg userid is allowed to interact
	if adminID == 0 || senderID != adminID {
		denied := tgbotapi.NewMessage(chatID, fmt.Sprintf("⛔ <b>Access Denied</b>\n\nYour Telegram User ID (<code>%d</code>) is not authorized to interact with this easy42 server.", senderID))
		denied.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(denied)
		return nil
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}

	command := fields[0]
	if strings.HasPrefix(command, "/") {
		// Strip bot username if invoked as /command@botname
		parts := strings.SplitN(command, "@", 2)
		command = parts[0]
	}
	command = strings.ToLower(command)
	args := fields[1:]

	switch command {
	case "/start":
		return s.handleStartCommand(chatID, serverURL)
	case "/help":
		return s.handleHelpCommand(chatID, serverURL)
	case "/status":
		return s.handleStatusCommand(chatID, serverURL)
	case "/nodes":
		return s.handleNodesCommand(chatID, serverURL)
	case "/node":
		return s.handleNodeCommand(chatID, serverURL, args)
	case "/ping":
		return s.handlePingCommand(chatID, args)
	case "/tcp_ping":
		return s.handleTCPPingCommand(chatID, args)
	default:
		reply := tgbotapi.NewMessage(chatID, "Unrecognized command. Type /status to check cluster status, or /help to see available commands.")
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

I am your easy42 overlay network monitoring and diagnostic assistant.

<b>Available Commands:</b>
• /status - Query current easy42 cluster overview and live metrics
• /nodes - Detailed node list, agent statuses, and resource usage
• /node [name] - Detailed info, network addresses, and live state of a node
• /ping [node] &lt;target&gt; - Run Looking Glass ICMP ping test
• /tcp_ping [node] &lt;host:port&gt; - Run Looking Glass TCP ping test
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
• <b>/nodes</b>: Summary list of cluster nodes with connection status and metrics.
• <b>/node [name-or-prefix]</b>: Detailed hardware metrics, network addresses, and live state for a specific node.
• <b>/ping [node] &lt;target&gt; [count]</b>: Execute Looking Glass ICMP ping test from a cluster node.
• <b>/tcp_ping [node] &lt;host:port&gt; [count]</b>: Execute Looking Glass TCP connectivity and latency test.
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
		sb.WriteString(fmt.Sprintf("\n🌐 Open easy42 Dashboard: %s", strings.TrimRight(serverURL, "/")))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, err = s.bot.Send(msg)
	return err
}

// NodeMatchResult holds the resolution state for node lookup by name or prefix
type NodeMatchResult struct {
	MatchedNode   *config.Node
	MultipleNodes []config.Node
	NotFound      bool
	NoArg         bool
}

// resolveNode resolves a node by exact match or unique prefix, or returns candidates
func resolveNode(nodes []config.Node, arg string) NodeMatchResult {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return NodeMatchResult{NoArg: true, MultipleNodes: nodes}
	}

	// Rule a: Exact match (case-insensitive for convenience, with exact-case check first)
	for i := range nodes {
		if nodes[i].Name == arg {
			return NodeMatchResult{MatchedNode: &nodes[i]}
		}
	}
	for i := range nodes {
		if strings.EqualFold(nodes[i].Name, arg) {
			return NodeMatchResult{MatchedNode: &nodes[i]}
		}
	}

	// Rule b: Prefix match
	var matches []config.Node
	for i := range nodes {
		if strings.HasPrefix(strings.ToLower(nodes[i].Name), strings.ToLower(arg)) {
			matches = append(matches, nodes[i])
		}
	}

	if len(matches) == 1 {
		return NodeMatchResult{MatchedNode: &matches[0]}
	}
	if len(matches) > 1 {
		return NodeMatchResult{MultipleNodes: matches}
	}

	return NodeMatchResult{NotFound: true}
}

// buildNodeSelectionKeyboard constructs inline keyboard buttons for nodes
// Payload of each inline button is "/node <node-name>"
func buildNodeSelectionKeyboard(nodes []config.Node) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	for _, n := range nodes {
		btn := tgbotapi.NewInlineKeyboardButtonData(n.Name, fmt.Sprintf("/node %s", n.Name))
		currentRow = append(currentRow, btn)
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// isLinkUp determines if a link is currently up/working
func isLinkUp(link config.Link, netState *config.NetworkState, liveMap map[string]config.NodeLiveStatus) bool {
	if link.IsManual() {
		fromLive, fromOk := liveMap[link.From.Name]
		toLive, toOk := liveMap[link.To.Name]
		return fromOk && fromLive.Connected && toOk && toLive.Connected
	}

	if netState == nil || netState.Nodes == nil {
		return false
	}

	now := time.Now()
	// Check from endpoint interface state
	if stNode, ok := netState.Nodes[link.From.Name]; ok && stNode.Interfaces != nil {
		if iface, ok := stNode.Interfaces[link.From.Interface]; ok {
			if iface.WorkingState == config.WorkingStateWorking {
				return true
			}
			if iface.LatestHandshake != nil && !iface.LatestHandshake.IsZero() && now.Sub(*iface.LatestHandshake) <= 3*time.Minute {
				return true
			}
		}
	}

	// Check to endpoint interface state
	if stNode, ok := netState.Nodes[link.To.Name]; ok && stNode.Interfaces != nil {
		if iface, ok := stNode.Interfaces[link.To.Interface]; ok {
			if iface.WorkingState == config.WorkingStateWorking {
				return true
			}
			if iface.LatestHandshake != nil && !iface.LatestHandshake.IsZero() && now.Sub(*iface.LatestHandshake) <= 3*time.Minute {
				return true
			}
		}
	}

	return false
}

func formatNodeDetail(node *config.Node, live *config.NodeLiveStatus, links []config.Link, netState *config.NetworkState, liveMap map[string]config.NodeLiveStatus, serverURL string) string {
	if node == nil {
		return ""
	}

	var sb strings.Builder

	// Header: Node Name + Status Icon
	var icon string
	var statusDesc string
	if live != nil && live.Connected {
		icon = "🟢"
		statusDesc = "Online / Connected"
	} else if live != nil && live.AgentInstalled {
		icon = "🔴"
		lastSeen := "never"
		if !live.LastSeen.IsZero() {
			lastSeen = time.Since(live.LastSeen).Truncate(time.Second).String() + " ago"
		}
		statusDesc = fmt.Sprintf("Offline (Last seen: %s)", lastSeen)
	} else if node.IsExternal {
		icon = "🌐"
		statusDesc = "External Peer (Unmanaged)"
	} else {
		icon = "⚪"
		statusDesc = "No Agent Installed"
	}

	sb.WriteString(fmt.Sprintf("%s <b>Node: %s</b>\n", icon, html.EscapeString(node.Name)))
	sb.WriteString(fmt.Sprintf("Status: <b>%s</b>\n", statusDesc))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")

	// Host & System Info
	sb.WriteString("💻 <b>System & Host:</b>\n")
	hostname := ""
	if live != nil && live.Hostname != "" {
		hostname = live.Hostname
	} else if node.Host != "" {
		hostname = node.Host
	} else {
		hostname = node.Name
	}
	sb.WriteString(fmt.Sprintf("• Hostname: <code>%s</code>\n", html.EscapeString(hostname)))

	if live != nil && live.OSInfo != "" {
		sb.WriteString(fmt.Sprintf("• OS: %s\n", html.EscapeString(live.OSInfo)))
	}

	agentVer := ""
	if live != nil && live.AgentVersion != "" {
		agentVer = live.AgentVersion
	} else if node.AgentVersion != "" {
		agentVer = node.AgentVersion
	}
	if agentVer != "" {
		sb.WriteString(fmt.Sprintf("• Agent Version: <code>%s</code>\n", html.EscapeString(agentVer)))
	}

	modeStr := "ssh"
	if node.IsAgentMode() {
		modeStr = "agent"
	}
	sb.WriteString(fmt.Sprintf("• Mode: <code>%s</code>\n", modeStr))

	if live != nil && live.Connected && live.UptimeSeconds > 0 {
		sb.WriteString(fmt.Sprintf("• Uptime: %s\n", formatUptime(live.UptimeSeconds)))
	}

	// Network & Addresses
	sb.WriteString("\n🌐 <b>Network & Routing:</b>\n")
	if node.ASN > 0 {
		sb.WriteString(fmt.Sprintf("• ASN: <code>%d</code>\n", node.ASN))
	}
	if node.IP != "" {
		sb.WriteString(fmt.Sprintf("• IPv4: <code>%s</code>\n", html.EscapeString(node.IP)))
	}
	if node.IP6 != "" {
		sb.WriteString(fmt.Sprintf("• IPv6: <code>%s</code>\n", html.EscapeString(node.IP6)))
	}
	if node.ExternalIP != "" && node.ExternalIP != node.IP {
		sb.WriteString(fmt.Sprintf("• External IPv4: <code>%s</code>\n", html.EscapeString(node.ExternalIP)))
	}
	if node.ExternalIP6 != "" && node.ExternalIP6 != node.IP6 {
		sb.WriteString(fmt.Sprintf("• External IPv6: <code>%s</code>\n", html.EscapeString(node.ExternalIP6)))
	}
	if node.Host != "" && !node.IsExternal {
		sb.WriteString(fmt.Sprintf("• Host/SSH: <code>%s</code>\n", html.EscapeString(node.Host)))
	}
	primaryIface := ""
	if live != nil && live.PrimaryInterface != "" {
		primaryIface = live.PrimaryInterface
	} else if node.Interface != "" {
		primaryIface = node.Interface
	}
	if primaryIface != "" {
		sb.WriteString(fmt.Sprintf("• Interface: <code>%s</code>\n", html.EscapeString(primaryIface)))
	}
	if len(node.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("• Tags: %s\n", html.EscapeString(strings.Join(node.Tags, ", "))))
	}
	if node.Caption != "" {
		sb.WriteString(fmt.Sprintf("• Caption: <i>%s</i>\n", html.EscapeString(node.Caption)))
	}
	if node.Description != "" {
		sb.WriteString(fmt.Sprintf("• Description: <i>%s</i>\n", html.EscapeString(node.Description)))
	}

	// Live Metrics (if available and connected)
	if live != nil && live.Connected && live.Metrics != nil {
		m := live.Metrics
		sb.WriteString("\n📊 <b>Live Telemetry:</b>\n")
		sb.WriteString(fmt.Sprintf("• CPU Usage: <b>%.1f%%</b>\n", m.CPUPercent))

		if m.MemoryTotalBytes > 0 {
			memPerc := float64(m.MemoryUsedBytes) / float64(m.MemoryTotalBytes) * 100.0
			sb.WriteString(fmt.Sprintf("• Memory: <b>%s / %s (%.0f%%)</b>\n",
				formatBytes(m.MemoryUsedBytes), formatBytes(m.MemoryTotalBytes), memPerc))
		}

		if m.Load1m > 0 || m.Load5m > 0 || m.Load15m > 0 {
			sb.WriteString(fmt.Sprintf("• Load Average: <b>%.2f, %.2f, %.2f</b>\n", m.Load1m, m.Load5m, m.Load15m))
		}

		sb.WriteString(fmt.Sprintf("• Traffic Rate: Rx <b>%s</b> | Tx <b>%s</b>\n",
			formatBytesRate(m.NetRxRate), formatBytesRate(m.NetTxRate)))
	}

	// Belonging Links & Live States
	type nodeLinkDisplay struct {
		peerName   string
		localIface string
		isManual   bool
		isUp       bool
	}

	var belonging []nodeLinkDisplay
	for _, l := range links {
		var peerName string
		var localIface string
		if l.From.Name == node.Name {
			peerName = l.To.Name
			localIface = l.From.Interface
		} else if l.To.Name == node.Name {
			peerName = l.From.Name
			localIface = l.To.Interface
		} else {
			continue
		}
		belonging = append(belonging, nodeLinkDisplay{
			peerName:   peerName,
			localIface: localIface,
			isManual:   l.IsManual(),
			isUp:       isLinkUp(l, netState, liveMap),
		})
	}

	sort.Slice(belonging, func(i, j int) bool {
		if belonging[i].peerName != belonging[j].peerName {
			return belonging[i].peerName < belonging[j].peerName
		}
		return belonging[i].localIface < belonging[j].localIface
	})

	sb.WriteString(fmt.Sprintf("\n🔗 <b>Links (%d):</b>\n", len(belonging)))
	if len(belonging) == 0 {
		sb.WriteString("• <i>No links configured</i>\n")
	} else {
		for _, lk := range belonging {
			statusIcon := "🟢"
			statusText := "up"
			if !lk.isUp {
				statusIcon = "🔴"
				statusText = "down"
			}
			ifaceDesc := ""
			if lk.localIface != "" {
				ifaceDesc = fmt.Sprintf(" (<code>%s</code>)", html.EscapeString(lk.localIface))
			} else if lk.isManual {
				ifaceDesc = " (<code>manual</code>)"
			}
			sb.WriteString(fmt.Sprintf("• %s %s ↔ <b>%s</b>%s\n", statusIcon, statusText, html.EscapeString(lk.peerName), ifaceDesc))
		}
	}

	// Link to Node Details Page
	detailsPath := fmt.Sprintf("/nodes/%s", node.Name)
	if serverURL != "" {
		baseURL := strings.TrimRight(serverURL, "/")
		sb.WriteString(fmt.Sprintf("\n🌐 Node Details: %s%s", baseURL, detailsPath))
	} else {
		sb.WriteString(fmt.Sprintf("\n🌐 Node Details: %s", detailsPath))
	}

	return sb.String()
}

// filterManagedNodes returns only internal managed nodes, ignoring external peers
func filterManagedNodes(nodes []config.Node) []config.Node {
	var managed []config.Node
	for _, n := range nodes {
		if !n.IsExternal {
			managed = append(managed, n)
		}
	}
	return managed
}

func (s *BotService) handleNodeCommand(chatID int64, serverURL string, args []string) error {
	if s.provider == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Cluster provider not available.")
		_, err := s.bot.Send(msg)
		return err
	}

	nodes := filterManagedNodes(s.provider.GetNodes())
	if len(nodes) == 0 {
		msg := tgbotapi.NewMessage(chatID, "ℹ️ No managed nodes in the easy42 cluster.")
		_, err := s.bot.Send(msg)
		return err
	}

	_, liveStatuses, _ := s.provider.GetFleetLiveStatus()
	liveMap := make(map[string]config.NodeLiveStatus, len(liveStatuses))
	for _, ls := range liveStatuses {
		liveMap[ls.Name] = ls
	}

	arg := ""
	if len(args) > 0 {
		arg = strings.TrimSpace(args[0])
	}

	res := resolveNode(nodes, arg)

	if res.MatchedNode != nil {
		var live *config.NodeLiveStatus
		if ls, ok := liveMap[res.MatchedNode.Name]; ok {
			live = &ls
		}
		links := s.provider.GetLinks()
		netState := s.provider.GetNetworkState()
		detailText := formatNodeDetail(res.MatchedNode, live, links, netState, liveMap, serverURL)
		msg := tgbotapi.NewMessage(chatID, detailText)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.DisableWebPagePreview = true
		_, err := s.bot.Send(msg)
		return err
	}

	if len(res.MultipleNodes) > 0 {
		var text string
		if res.NoArg {
			text = fmt.Sprintf("🖥️ <b>easy42 Cluster Nodes (%d total):</b>\n━━━━━━━━━━━━━━━━━━━━\nSelect a node below to view detailed info and state:", len(nodes))
		} else {
			text = fmt.Sprintf("🔍 <b>Multiple Nodes Matched:</b>\n━━━━━━━━━━━━━━━━━━━━\nMultiple nodes match prefix <code>%s</code>. Select a node below:", html.EscapeString(arg))
		}
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		markup := buildNodeSelectionKeyboard(res.MultipleNodes)
		msg.ReplyMarkup = markup
		_, err := s.bot.Send(msg)
		return err
	}

	// Not found
	text := fmt.Sprintf("❌ <b>Node Not Found</b>\n━━━━━━━━━━━━━━━━━━━━\nNo node matching <code>%s</code> was found in easy42 cluster.\n\nType <code>/node</code> to browse all cluster nodes.", html.EscapeString(arg))
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := s.bot.Send(msg)
	return err
}

func pingHelpMessage() string {
	return `ℹ️ <b>Ping Command Usage</b>
━━━━━━━━━━━━━━━━━━━━
Usage: <code>/ping [node] &lt;target&gt; [count]</code>

<b>Arguments:</b>
• <code>node</code> (optional): Cluster node name or prefix to run ping from. If omitted, the first available node is used.
• <code>target</code> (required): Destination IPv4, IPv6, or hostname (e.g. <code>1.1.1.1</code>, <code>example.com</code>).
• <code>count</code> (optional): Number of ICMP packets to send (1-20, default: 4).

<b>Examples:</b>
• <code>/ping 1.1.1.1</code>
• <code>/ping node1 1.0.0.1</code>
• <code>/ping node1 example.com 5</code>`
}

func tcpPingHelpMessage() string {
	return `ℹ️ <b>TCP Ping Command Usage</b>
━━━━━━━━━━━━━━━━━━━━
Usage: <code>/tcp_ping [node] &lt;host:port&gt; [count]</code>

<b>Arguments:</b>
• <code>node</code> (optional): Cluster node name or prefix to run TCP ping from. If omitted, the first available node with an active agent is used.
• <code>host:port</code> (required): Target destination in <code>&lt;host&gt;:&lt;port&gt;</code> format (e.g. <code>1.1.1.1:80</code>, <code>example.com:443</code>).
• <code>count</code> (optional): Number of connection probes (1-20, default: 4).

<b>Examples:</b>
• <code>/tcp_ping 1.1.1.1:80</code>
• <code>/tcp_ping node1 example.com:443</code>
• <code>/tcp_ping node1 1.1.1.1:443 5</code>`
}

func formatPingResult(nodeName string, target string, nr lookingglass.NodeResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏓 <b>Ping Result: %s</b>\n", html.EscapeString(target)))
	sb.WriteString(fmt.Sprintf("📍 Source Node: <b>%s</b>\n━━━━━━━━━━━━━━━━━━━━\n", html.EscapeString(nodeName)))

	if nr.Error != "" {
		sb.WriteString(fmt.Sprintf("❌ <b>Error:</b> %s\n", html.EscapeString(nr.Error)))
		if nr.RawOutput != "" {
			sb.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(nr.RawOutput)))
		}
		return sb.String()
	}

	if pr, ok := nr.Parsed.(lookingglass.PingResult); ok && pr.PacketsSent > 0 {
		hostStr := pr.Host
		if pr.IP != "" && pr.IP != pr.Host {
			hostStr = fmt.Sprintf("%s (%s)", pr.Host, pr.IP)
		}
		sb.WriteString(fmt.Sprintf("• Host: <code>%s</code>\n", html.EscapeString(hostStr)))
		sb.WriteString(fmt.Sprintf("• Packets: %d transmitted, %d received (<b>%.0f%% loss</b>)\n",
			pr.PacketsSent, pr.PacketsReceived, pr.PacketLossPct))
		if pr.AvgRTT > 0 {
			sb.WriteString(fmt.Sprintf("• RTT: min <b>%.2fms</b> | avg <b>%.2fms</b> | max <b>%.2fms</b>",
				pr.MinRTT, pr.AvgRTT, pr.MaxRTT))
			if pr.MdevRTT > 0 {
				sb.WriteString(fmt.Sprintf(" | mdev <b>%.2fms</b>", pr.MdevRTT))
			}
			sb.WriteString("\n")
		}
	} else if nr.RawOutput != "" {
		sb.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(strings.TrimSpace(nr.RawOutput))))
	} else {
		sb.WriteString("<i>No output returned.</i>\n")
	}

	return sb.String()
}

func formatTCPPingResult(nodeName string, target string, nr lookingglass.NodeResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔌 <b>TCP Ping Result: %s</b>\n", html.EscapeString(target)))
	sb.WriteString(fmt.Sprintf("📍 Source Node: <b>%s</b>\n━━━━━━━━━━━━━━━━━━━━\n", html.EscapeString(nodeName)))

	if nr.Error != "" {
		sb.WriteString(fmt.Sprintf("❌ <b>Error:</b> %s\n", html.EscapeString(nr.Error)))
		if nr.RawOutput != "" {
			sb.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(nr.RawOutput)))
		}
		return sb.String()
	}

	if tr, ok := nr.Parsed.(*lookingglass.TCPPingResult); ok && tr != nil && tr.PacketsSent > 0 {
		addrStr := tr.Target
		if tr.ResolvedAddr != "" && tr.ResolvedAddr != tr.Target {
			addrStr = fmt.Sprintf("%s (%s)", tr.Target, tr.ResolvedAddr)
		}
		sb.WriteString(fmt.Sprintf("• Target: <code>%s</code>\n", html.EscapeString(addrStr)))
		sb.WriteString(fmt.Sprintf("• Probes: %d sent, %d connected (<b>%.0f%% loss</b>)\n",
			tr.PacketsSent, tr.PacketsReceived, tr.PacketLossPct))
		if tr.AvgRTT > 0 {
			sb.WriteString(fmt.Sprintf("• RTT: min <b>%.2fms</b> | avg <b>%.2fms</b> | max <b>%.2fms</b>",
				tr.MinRTT, tr.AvgRTT, tr.MaxRTT))
			if tr.MdevRTT > 0 {
				sb.WriteString(fmt.Sprintf(" | mdev <b>%.2fms</b>", tr.MdevRTT))
			}
			sb.WriteString("\n")
		}
	} else if nr.RawOutput != "" {
		sb.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(strings.TrimSpace(nr.RawOutput))))
	} else {
		sb.WriteString("<i>No output returned.</i>\n")
	}

	return sb.String()
}

func (s *BotService) handlePingCommand(chatID int64, args []string) error {
	if s.provider == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Cluster provider not available.")
		_, err := s.bot.Send(msg)
		return err
	}

	if len(args) == 0 {
		msg := tgbotapi.NewMessage(chatID, pingHelpMessage())
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := s.bot.Send(msg)
		return err
	}

	nodes := filterManagedNodes(s.provider.GetNodes())
	_, liveStatuses, _ := s.provider.GetFleetLiveStatus()

	var selectedNode string
	var target string
	countStr := "4"

	// Resolve target and node from args
	matchRes := resolveNode(nodes, args[0])
	if matchRes.MatchedNode != nil {
		selectedNode = matchRes.MatchedNode.Name
		if len(args) < 2 {
			errText := "⚠️ <b>Missing Target:</b> A target host or IP is required.\n\n" + pingHelpMessage()
			msg := tgbotapi.NewMessage(chatID, errText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err := s.bot.Send(msg)
			return err
		}
		target = args[1]
		if len(args) >= 3 {
			countStr = args[2]
		}
	} else if len(args) >= 2 && !isNumber(args[1]) {
		if len(matchRes.MultipleNodes) > 1 {
			errText := fmt.Sprintf("⚠️ Multiple nodes match prefix <code>%s</code>. Please specify the complete node name.\n\n%s",
				html.EscapeString(args[0]), pingHelpMessage())
			msg := tgbotapi.NewMessage(chatID, errText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err := s.bot.Send(msg)
			return err
		}
		errText := fmt.Sprintf("⚠️ Node <code>%s</code> not found in cluster.\n\n%s",
			html.EscapeString(args[0]), pingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := s.bot.Send(msg)
		return err
	} else {
		target = args[0]
		if len(args) >= 2 {
			countStr = args[1]
		}
		// Pick first available online node
		for _, ls := range liveStatuses {
			if ls.Connected {
				selectedNode = ls.Name
				break
			}
		}
		if selectedNode == "" {
			for _, n := range nodes {
				if !n.IsExternal {
					selectedNode = n.Name
					break
				}
			}
		}
		if selectedNode == "" {
			msg := tgbotapi.NewMessage(chatID, "⚠️ No available cluster nodes found to execute ping.")
			_, err := s.bot.Send(msg)
			return err
		}
	}

	// Validate target
	specTarget := config.TaskParam{
		Key:   "target",
		Label: "Target Host or IP",
		Type:  config.ParamTypeIPOrCIDR,
	}
	cleanTarget, err := lookingglass.SanitizeParam(specTarget, target)
	if err != nil {
		errText := fmt.Sprintf("⚠️ <b>Invalid Target:</b> %v\n\n%s", err, pingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	// Validate count
	specCount := config.TaskParam{
		Key:          "count",
		Label:        "Packet Count",
		Type:         config.ParamTypeNumber,
		DefaultValue: "4",
	}
	cleanCount, err := lookingglass.SanitizeParam(specCount, countStr)
	if err != nil {
		errText := fmt.Sprintf("⚠️ <b>Invalid Packet Count:</b> %v\n\n%s", err, pingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = s.bot.Send(typing)

	req := lookingglass.RunRequest{
		Nodes:      []string{selectedNode},
		TaskID:     "ping",
		Params:     map[string]string{"target": cleanTarget, "count": cleanCount},
		TimeoutSec: 20,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	resp, err := s.provider.RunLookingGlass(ctx, req)
	if err != nil {
		errMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Failed to execute ping on node <b>%s</b>: %v", html.EscapeString(selectedNode), err))
		errMsg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(errMsg)
		return sErr
	}

	nodeRes, ok := resp.Results[selectedNode]
	if !ok {
		errMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ No result returned from node <b>%s</b>.", html.EscapeString(selectedNode)))
		errMsg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(errMsg)
		return sErr
	}

	resultText := formatPingResult(selectedNode, cleanTarget, nodeRes)
	resMsg := tgbotapi.NewMessage(chatID, resultText)
	resMsg.ParseMode = tgbotapi.ModeHTML
	_, sErr := s.bot.Send(resMsg)
	return sErr
}

func (s *BotService) handleTCPPingCommand(chatID int64, args []string) error {
	if s.provider == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Cluster provider not available.")
		_, err := s.bot.Send(msg)
		return err
	}

	if len(args) == 0 {
		msg := tgbotapi.NewMessage(chatID, tcpPingHelpMessage())
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := s.bot.Send(msg)
		return err
	}

	nodes := filterManagedNodes(s.provider.GetNodes())
	_, liveStatuses, _ := s.provider.GetFleetLiveStatus()

	var selectedNode string
	var target string
	countStr := "4"

	matchRes := resolveNode(nodes, args[0])
	if matchRes.MatchedNode != nil {
		selectedNode = matchRes.MatchedNode.Name
		if len(args) < 2 {
			errText := "⚠️ <b>Missing Target:</b> A destination in <code>&lt;host&gt;:&lt;port&gt;</code> format is required.\n\n" + tcpPingHelpMessage()
			msg := tgbotapi.NewMessage(chatID, errText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err := s.bot.Send(msg)
			return err
		}
		target = args[1]
		if len(args) >= 3 {
			countStr = args[2]
		}
	} else if len(args) >= 2 && !isNumber(args[1]) {
		if len(matchRes.MultipleNodes) > 1 {
			errText := fmt.Sprintf("⚠️ Multiple nodes match prefix <code>%s</code>. Please specify the complete node name.\n\n%s",
				html.EscapeString(args[0]), tcpPingHelpMessage())
			msg := tgbotapi.NewMessage(chatID, errText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err := s.bot.Send(msg)
			return err
		}
		errText := fmt.Sprintf("⚠️ Node <code>%s</code> not found in cluster.\n\n%s",
			html.EscapeString(args[0]), tcpPingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := s.bot.Send(msg)
		return err
	} else {
		target = args[0]
		if len(args) >= 2 {
			countStr = args[1]
		}
		// Pick first available online node with agent
		for _, ls := range liveStatuses {
			if ls.Connected && (ls.AgentInstalled || ls.AgentVersion != "") {
				selectedNode = ls.Name
				break
			}
		}
		if selectedNode == "" {
			for _, n := range nodes {
				if n.IsAgentMode() && !n.IsExternal {
					selectedNode = n.Name
					break
				}
			}
		}
		if selectedNode == "" {
			msg := tgbotapi.NewMessage(chatID, "⚠️ No active easy42 agent node found in cluster to execute TCP ping.")
			_, err := s.bot.Send(msg)
			return err
		}
	}

	// Validate target
	specTarget := config.TaskParam{
		Key:   "target",
		Label: "Target (<host>:<port>)",
		Type:  config.ParamTypeHostPort,
	}
	cleanTarget, err := lookingglass.SanitizeParam(specTarget, target)
	if err != nil {
		errText := fmt.Sprintf("⚠️ <b>Invalid Target:</b> %v\n\n%s", err, tcpPingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	// Validate count
	specCount := config.TaskParam{
		Key:          "count",
		Label:        "Probe Count",
		Type:         config.ParamTypeNumber,
		DefaultValue: "4",
	}
	cleanCount, err := lookingglass.SanitizeParam(specCount, countStr)
	if err != nil {
		errText := fmt.Sprintf("⚠️ <b>Invalid Probe Count:</b> %v\n\n%s", err, tcpPingHelpMessage())
		msg := tgbotapi.NewMessage(chatID, errText)
		msg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(msg)
		return sErr
	}

	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = s.bot.Send(typing)

	req := lookingglass.RunRequest{
		Nodes:      []string{selectedNode},
		TaskID:     "tcp_ping",
		Params:     map[string]string{"target": cleanTarget, "count": cleanCount},
		TimeoutSec: 15,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := s.provider.RunLookingGlass(ctx, req)
	if err != nil {
		errMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Failed to execute TCP ping on node <b>%s</b>: %v", html.EscapeString(selectedNode), err))
		errMsg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(errMsg)
		return sErr
	}

	nodeRes, ok := resp.Results[selectedNode]
	if !ok {
		errMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ No result returned from node <b>%s</b>.", html.EscapeString(selectedNode)))
		errMsg.ParseMode = tgbotapi.ModeHTML
		_, sErr := s.bot.Send(errMsg)
		return sErr
	}

	resultText := formatTCPPingResult(selectedNode, cleanTarget, nodeRes)
	resMsg := tgbotapi.NewMessage(chatID, resultText)
	resMsg.ParseMode = tgbotapi.ModeHTML
	_, sErr := s.bot.Send(resMsg)
	return sErr
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func formatBytes(b uint64) string {
	if b == 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
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

	if s.nodeStates == nil {
		s.nodeStates = make(map[string]*nodeDebounceState)
	}
	if s.prevNodeOnline == nil {
		s.prevNodeOnline = make(map[string]bool)
	}

	debounceLimit := s.debounceCount
	if debounceLimit <= 0 {
		debounceLimit = 2
	}

	if s.monitorStartedAt.IsZero() {
		s.monitorStartedAt = time.Now()
	}

	isStartupGrace := s.startupGracePeriod > 0 && time.Since(s.monitorStartedAt) < s.startupGracePeriod

	if !s.monitorInitDone {
		// Initial baseline recording - do not send alerts on first run
		for _, n := range nodes {
			s.nodeStates[n.Name] = &nodeDebounceState{
				notifiedOnline: n.Connected,
				pendingOnline:  n.Connected,
				pendingCount:   0,
			}
			s.prevNodeOnline[n.Name] = n.Connected
		}
		s.monitorInitDone = true
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, n := range nodes {
		state, existed := s.nodeStates[n.Name]
		if !existed {
			// Newly discovered node
			s.nodeStates[n.Name] = &nodeDebounceState{
				notifiedOnline: n.Connected,
				pendingOnline:  n.Connected,
				pendingCount:   0,
			}
			s.prevNodeOnline[n.Name] = n.Connected
			continue
		}

		currentOnline := n.Connected

		// If current online state matches the last notified/confirmed state,
		// any ongoing pending transition flap is cancelled.
		if currentOnline == state.notifiedOnline {
			state.pendingOnline = currentOnline
			state.pendingCount = 0
			s.prevNodeOnline[n.Name] = currentOnline
			continue
		}

		// State differs from notifiedOnline!
		if currentOnline == state.pendingOnline {
			state.pendingCount++
		} else {
			// State switched to a different pending state
			state.pendingOnline = currentOnline
			state.pendingSince = time.Now()
			state.pendingCount = 1
		}

		// Check if pending state has stabilized
		if state.pendingCount >= debounceLimit {
			wasOnline := state.notifiedOnline
			state.notifiedOnline = currentOnline
			state.pendingCount = 0
			s.prevNodeOnline[n.Name] = currentOnline

			// If transitioning to Online during the startup grace period,
			// suppress the alert because the server just started/restarted and nodes are naturally reconnecting.
			if !wasOnline && currentOnline && isStartupGrace {
				continue
			}

			if wasOnline && !currentOnline {
				// Node went OFFLINE
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
			} else if !wasOnline && currentOnline {
				// Node went BACK ONLINE
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
