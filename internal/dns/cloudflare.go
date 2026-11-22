package dns

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy42/internal/apps"
	"easy42/internal/config"
)

const (
	DefaultCloudflareAPIURL = "https://api.cloudflare.com/client/v4"
	easy42RecordComment     = "easy42 managed"
)

// DNSRecord represents a Cloudflare DNS record
type DNSRecord struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Content  string         `json:"content,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	Priority *int           `json:"priority,omitempty"`
	TTL      int            `json:"ttl"`
	Proxied  bool           `json:"proxied"`
	Comment  string         `json:"comment,omitempty"`
	ZoneID   string         `json:"zone_id,omitempty"`
	ZoneName string         `json:"zone_name,omitempty"`
}

// SyncResult details the outcome of a DNS sync operation
type SyncResult struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Deleted int      `json:"deleted"`
	Ignored int      `json:"ignored"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message,omitempty"`
}

type cfResponse[T any] struct {
	Success  bool      `json:"success"`
	Errors   []cfError `json:"errors"`
	Messages []string  `json:"messages"`
	Result   T         `json:"result"`
	Info     *cfInfo   `json:"result_info,omitempty"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
}

// Client wraps Cloudflare API interactions for DNS records
type Client struct {
	httpClient *http.Client
	baseURL    string
	zoneID     string
	apiToken   string
	baseDomain string
	mu         sync.Mutex
	zoneName   string
}

// NewClient creates a new Cloudflare DNS client
func NewClient(zoneID, apiToken, baseDomain string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    DefaultCloudflareAPIURL,
		zoneID:     strings.TrimSpace(zoneID),
		apiToken:   strings.TrimSpace(apiToken),
		baseDomain: NormalizeDomain(baseDomain),
	}
}

// WithBaseURL overrides the API base URL (useful for testing)
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = strings.TrimRight(url, "/")
	return c
}

// WithHTTPClient overrides the HTTP client
func (c *Client) WithHTTPClient(httpClient *http.Client) *Client {
	c.httpClient = httpClient
	return c
}

// NormalizeDomain cleans up domain strings (trims spaces, http prefix, trailing/leading dots, lowercases)
func NormalizeDomain(domain string) string {
	d := strings.TrimSpace(strings.ToLower(domain))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	if idx := strings.Index(d, "/"); idx != -1 {
		d = d[:idx]
	}
	return strings.Trim(d, ".")
}

// CleanIP strips CIDR masks and trims spaces
func CleanIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if idx := strings.Index(ip, "/"); idx != -1 {
		ip = strings.TrimSpace(ip[:idx])
	}
	return ip
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, body any, out any) (*cfInfo, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var cfResp cfResponse[json.RawMessage]
	if err := json.Unmarshal(respBytes, &cfResp); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("cloudflare API HTTP error %d: %s", resp.StatusCode, string(respBytes))
		}
		return nil, fmt.Errorf("failed to parse cloudflare response: %w", err)
	}

	if !cfResp.Success || resp.StatusCode >= 400 {
		var errMsgs []string
		for _, e := range cfResp.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
		}
		if len(errMsgs) == 0 {
			errMsgs = append(errMsgs, fmt.Sprintf("HTTP status %d", resp.StatusCode))
		}
		return nil, fmt.Errorf("cloudflare API error: %s", strings.Join(errMsgs, ", "))
	}

	if out != nil && len(cfResp.Result) > 0 {
		if err := json.Unmarshal(cfResp.Result, out); err != nil {
			return nil, fmt.Errorf("failed to decode result: %w", err)
		}
	}

	return cfResp.Info, nil
}

// GetZoneName fetches and caches the Cloudflare zone's root domain name (e.g. "example.com")
func (c *Client) GetZoneName(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.zoneName
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	endpoint := fmt.Sprintf("/zones/%s", c.zoneID)
	var zone struct {
		Name string `json:"name"`
	}
	if _, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, &zone); err != nil {
		return "", err
	}
	name := NormalizeDomain(zone.Name)
	if name == "" {
		return "", fmt.Errorf("cloudflare zone %s returned empty name", c.zoneID)
	}

	c.mu.Lock()
	c.zoneName = name
	c.mu.Unlock()
	return name, nil
}

// ListRecords lists DNS records in the zone, optionally filtered by name
func (c *Client) ListRecords(ctx context.Context, nameFilter string) ([]DNSRecord, error) {
	var allRecords []DNSRecord
	page := 1
	perPage := 100

	for {
		params := url.Values{}
		params.Set("per_page", fmt.Sprintf("%d", perPage))
		params.Set("page", fmt.Sprintf("%d", page))
		if nameFilter != "" {
			params.Set("name", strings.ToLower(strings.TrimSpace(nameFilter)))
		}

		endpoint := fmt.Sprintf("/zones/%s/dns_records?%s", c.zoneID, params.Encode())
		var pageRecords []DNSRecord
		info, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, &pageRecords)
		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, pageRecords...)

		if info == nil || len(pageRecords) < perPage || page >= info.TotalPages {
			break
		}
		page++
	}

	return allRecords, nil
}

func buildRecordPayload(rec DNSRecord) map[string]any {
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 1 // Automatic TTL
	}
	payload := map[string]any{
		"type":    rec.Type,
		"name":    rec.Name,
		"ttl":     ttl,
		"proxied": rec.Proxied,
		"comment": easy42RecordComment,
	}
	if len(rec.Data) > 0 {
		payload["data"] = rec.Data
	} else {
		payload["content"] = rec.Content
	}
	return payload
}

// CreateRecord creates a new DNS record
func (c *Client) CreateRecord(ctx context.Context, rec DNSRecord) (*DNSRecord, error) {
	endpoint := fmt.Sprintf("/zones/%s/dns_records", c.zoneID)
	payload := buildRecordPayload(rec)

	var created DNSRecord
	_, err := c.doRequest(ctx, http.MethodPost, endpoint, payload, &created)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateRecord updates an existing DNS record
func (c *Client) UpdateRecord(ctx context.Context, id string, rec DNSRecord) (*DNSRecord, error) {
	endpoint := fmt.Sprintf("/zones/%s/dns_records/%s", c.zoneID, id)
	payload := buildRecordPayload(rec)

	var updated DNSRecord
	_, err := c.doRequest(ctx, http.MethodPut, endpoint, payload, &updated)
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteRecord deletes a DNS record by ID
func (c *Client) DeleteRecord(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/zones/%s/dns_records/%s", c.zoneID, id)
	_, err := c.doRequest(ctx, http.MethodDelete, endpoint, nil, nil)
	return err
}

// ExpectedRecordsForNode calculates the expected DNS records for a given node
func ExpectedRecordsForNode(node config.Node, baseDomain string, publishIPv6OwnName bool) []DNSRecord {
	if !node.HasRoutableMainIP() {
		return nil
	}
	var records []DNSRecord

	nodeName := strings.ToLower(strings.TrimSpace(node.Name))
	if nodeName == "" {
		return nil
	}
	baseDomain = NormalizeDomain(baseDomain)
	if baseDomain == "" {
		return nil
	}

	ipv4 := CleanIP(node.IP)
	ipv6 := CleanIP(node.IP6)

	// Validate IPv4
	if ipv4 != "" {
		if parsed := net.ParseIP(ipv4); parsed != nil && parsed.To4() != nil {
			ipStr := parsed.String()
			records = append(records, DNSRecord{
				Type:    "A",
				Name:    fmt.Sprintf("%s.%s", nodeName, baseDomain),
				Content: ipStr,
				TTL:     1,
				Proxied: false,
				Comment: easy42RecordComment,
			})
			records = append(records, DNSRecord{
				Type:    "A",
				Name:    fmt.Sprintf("*.%s.%s", nodeName, baseDomain),
				Content: ipStr,
				TTL:     1,
				Proxied: false,
				Comment: easy42RecordComment,
			})
		}
	}

	// Validate IPv6
	if ipv6 != "" {
		if parsed := net.ParseIP(ipv6); parsed != nil && parsed.To4() == nil {
			ip6Str := parsed.String()
			if publishIPv6OwnName {
				records = append(records, DNSRecord{
					Type:    "AAAA",
					Name:    fmt.Sprintf("%s6.%s", nodeName, baseDomain),
					Content: ip6Str,
					TTL:     1,
					Proxied: false,
					Comment: easy42RecordComment,
				})
			} else {
				records = append(records, DNSRecord{
					Type:    "AAAA",
					Name:    fmt.Sprintf("%s.%s", nodeName, baseDomain),
					Content: ip6Str,
					TTL:     1,
					Proxied: false,
					Comment: easy42RecordComment,
				})
				records = append(records, DNSRecord{
					Type:    "AAAA",
					Name:    fmt.Sprintf("*.%s.%s", nodeName, baseDomain),
					Content: ip6Str,
					TTL:     1,
					Proxied: false,
					Comment: easy42RecordComment,
				})
			}
		}
	}

	return records
}

// IPMatches checks if two IP representations evaluate to the exact same IP
func IPMatches(ip1, ip2 string) bool {
	p1 := net.ParseIP(CleanIP(ip1))
	p2 := net.ParseIP(CleanIP(ip2))
	if p1 != nil && p2 != nil {
		return p1.Equal(p2)
	}
	return strings.EqualFold(strings.TrimSpace(ip1), strings.TrimSpace(ip2))
}

func normalizeTTL(ttl int) int {
	if ttl <= 0 {
		return 1
	}
	return ttl
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	default:
		return 0, false
	}
}

func parseSRVValue(val string) (map[string]any, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, fmt.Errorf("SRV value cannot be empty")
	}
	if strings.HasPrefix(val, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(val), &raw); err != nil {
			return nil, fmt.Errorf("invalid SRV JSON value: %w", err)
		}
		targetRaw, ok := raw["target"].(string)
		if !ok || strings.Trim(strings.TrimSpace(targetRaw), ".") == "" {
			return nil, fmt.Errorf("SRV JSON value requires non-empty 'target'")
		}
		data := make(map[string]any, len(raw))
		for k, v := range raw {
			data[k] = v
		}
		data["target"] = strings.ToLower(strings.Trim(strings.TrimSpace(targetRaw), "."))
		for _, numKey := range []string{"priority", "weight", "port"} {
			if rawVal, exists := raw[numKey]; exists {
				if iv, ok := toInt(rawVal); ok {
					data[numKey] = iv
				} else {
					return nil, fmt.Errorf("SRV JSON field '%s' must be an integer", numKey)
				}
			}
		}
		if _, exists := data["port"]; !exists {
			return nil, fmt.Errorf("SRV JSON value requires 'port'")
		}
		if _, exists := data["priority"]; !exists {
			data["priority"] = 0
		}
		if _, exists := data["weight"]; !exists {
			data["weight"] = 0
		}
		return data, nil
	}

	fields := strings.Fields(val)
	if len(fields) != 4 {
		return nil, fmt.Errorf("invalid SRV value '%s': expected JSON object or '<priority> <weight> <port> <target>'", val)
	}
	priority, err := strconv.Atoi(fields[0])
	if err != nil || priority < 0 {
		return nil, fmt.Errorf("invalid SRV priority '%s'", fields[0])
	}
	weight, err := strconv.Atoi(fields[1])
	if err != nil || weight < 0 {
		return nil, fmt.Errorf("invalid SRV weight '%s'", fields[1])
	}
	port, err := strconv.Atoi(fields[2])
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SRV port '%s'", fields[2])
	}
	target := strings.ToLower(strings.Trim(strings.TrimSpace(fields[3]), "."))
	if target == "" {
		return nil, fmt.Errorf("SRV target cannot be empty")
	}
	return map[string]any{
		"priority": priority,
		"weight":   weight,
		"port":     port,
		"target":   target,
	}, nil
}

func extractSRVTuple(rec DNSRecord) (priority, weight, port int, target string, ok bool) {
	if len(rec.Data) > 0 {
		p, okP := toInt(rec.Data["priority"])
		if !okP && rec.Priority != nil {
			p = *rec.Priority
			okP = true
		}
		w, okW := toInt(rec.Data["weight"])
		pt, okPt := toInt(rec.Data["port"])
		t, okT := rec.Data["target"].(string)
		if okW && okPt && okT {
			return p, w, pt, strings.ToLower(strings.Trim(strings.TrimSpace(t), ".")), okP || rec.Priority == nil
		}
	}
	// Fallback: Cloudflare content string "<weight> <port> <target>" with rec.Priority, or "<priority> <weight> <port> <target>"
	fields := strings.Fields(rec.Content)
	if len(fields) == 3 && rec.Priority != nil {
		w, errW := strconv.Atoi(fields[0])
		pt, errPt := strconv.Atoi(fields[1])
		if errW == nil && errPt == nil {
			return *rec.Priority, w, pt, strings.ToLower(strings.Trim(fields[2], ".")), true
		}
	} else if len(fields) == 4 {
		p, errP := strconv.Atoi(fields[0])
		w, errW := strconv.Atoi(fields[1])
		pt, errPt := strconv.Atoi(fields[2])
		if errP == nil && errW == nil && errPt == nil {
			return p, w, pt, strings.ToLower(strings.Trim(fields[3], ".")), true
		}
	}
	return 0, 0, 0, "", false
}

func srvDataEqual(a, b DNSRecord) bool {
	pa, wa, pta, ta, okA := extractSRVTuple(a)
	pb, wb, ptb, tb, okB := extractSRVTuple(b)
	if !okA || !okB {
		return false
	}
	return pa == pb && wa == wb && pta == ptb && ta == tb
}

func sshfpAlgorithmForKey(alg string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(alg)) {
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return 3, true
	case "ssh-ed25519":
		return 4, true
	case "ssh-ed448":
		return 6, true
	default:
		return 0, false
	}
}

func sshfpFingerprintSHA256(base64Key string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Key))
	if err != nil || len(raw) == 0 {
		return "", fmt.Errorf("invalid base64 SSH key")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func extractSSHFPTuple(rec DNSRecord) (algorithm, fpType int, fingerprint string, ok bool) {
	if len(rec.Data) > 0 {
		alg, okA := toInt(rec.Data["algorithm"])
		ft, okT := toInt(rec.Data["type"])
		fp, okF := rec.Data["fingerprint"].(string)
		if okA && okT && okF {
			return alg, ft, strings.ToLower(strings.TrimSpace(fp)), true
		}
	}
	fields := strings.Fields(rec.Content)
	if len(fields) == 3 {
		alg, errA := strconv.Atoi(fields[0])
		ft, errT := strconv.Atoi(fields[1])
		if errA == nil && errT == nil {
			return alg, ft, strings.ToLower(strings.TrimSpace(fields[2])), true
		}
	}
	return 0, 0, "", false
}

func sshfpDataEqual(a, b DNSRecord) bool {
	aa, fta, fpa, okA := extractSSHFPTuple(a)
	ab, ftb, fpb, okB := extractSSHFPTuple(b)
	if !okA || !okB {
		return false
	}
	return aa == ab && fta == ftb && fpa == fpb
}

func recordsMatch(existing, expected DNSRecord) bool {
	if normalizeTTL(existing.TTL) != normalizeTTL(expected.TTL) || existing.Proxied != expected.Proxied {
		return false
	}
	switch strings.ToUpper(expected.Type) {
	case "A", "AAAA":
		return IPMatches(existing.Content, expected.Content)
	case "CNAME":
		return strings.EqualFold(strings.Trim(strings.TrimSpace(existing.Content), "."), strings.Trim(strings.TrimSpace(expected.Content), "."))
	case "TXT":
		return strings.TrimSpace(existing.Content) == strings.TrimSpace(expected.Content)
	case "SRV":
		return srvDataEqual(existing, expected)
	case "SSHFP":
		return sshfpDataEqual(existing, expected)
	default:
		return strings.TrimSpace(existing.Content) == strings.TrimSpace(expected.Content)
	}
}

// ExpectedSSHFPRecordsForNode evaluates SSHFP DNS record rules for a single node using its agent-collected host keys
func ExpectedSSHFPRecordsForNode(node config.Node, rules []config.DNSSSHFPRule, baseDomain string, nodeKeys []config.SSHPublicKey) []DNSRecord {
	if len(rules) == 0 || len(nodeKeys) == 0 {
		return nil
	}
	nodeName := strings.ToLower(strings.TrimSpace(node.Name))
	baseDomain = NormalizeDomain(baseDomain)
	if nodeName == "" || baseDomain == "" {
		return nil
	}

	allowedAlgs := make(map[int]bool)
	for _, rule := range rules {
		if !config.NodeMatchesSelector(node, rule.TargetNodes) {
			continue
		}
		for _, alg := range rule.Algorithms {
			if alg == 3 || alg == 4 || alg == 6 {
				allowedAlgs[alg] = true
			}
		}
	}
	if len(allowedAlgs) == 0 {
		return nil
	}

	fqdn := fmt.Sprintf("%s.%s", nodeName, baseDomain)
	var records []DNSRecord
	for _, k := range nodeKeys {
		if !k.IsHostKey() {
			continue
		}
		algNum, ok := sshfpAlgorithmForKey(k.Algorithm)
		if !ok || !allowedAlgs[algNum] {
			continue
		}
		fp, err := sshfpFingerprintSHA256(k.Key)
		if err != nil {
			continue
		}
		records = append(records, DNSRecord{
			Type:    "SSHFP",
			Name:    fqdn,
			TTL:     1,
			Proxied: false,
			Comment: easy42RecordComment,
			Data: map[string]any{
				"algorithm":   algNum,
				"type":        2, // SHA-256
				"fingerprint": fp,
			},
		})
	}
	return records
}

// ExpectedCustomRecordsForNode evaluates custom DNS record rules for a single node
func ExpectedCustomRecordsForNode(node config.Node, rules []config.DNSCustomRecordRule, baseDomain, rootDomain string, globalVars map[string]string) ([]DNSRecord, []string) {
	if len(rules) == 0 {
		return nil, nil
	}

	baseDomain = NormalizeDomain(baseDomain)
	rootDomain = NormalizeDomain(rootDomain)

	contextVars := config.BuildNodeContextVars(node)
	evalVars := make(map[string]string, len(contextVars)+2)
	for k, v := range contextVars {
		evalVars[k] = v
	}
	if baseDomain != "" {
		evalVars["BASE"] = baseDomain
	}
	if rootDomain != "" {
		evalVars["ROOT"] = rootDomain
	}

	var records []DNSRecord
	var errs []string

	for i, rule := range rules {
		if !config.NodeMatchesSelector(node, rule.TargetNodes) {
			continue
		}

		recType := strings.ToUpper(strings.TrimSpace(rule.Type))
		renderedName := apps.InterpolateVariables(rule.Name, globalVars, evalVars)
		name := strings.ToLower(strings.Trim(strings.TrimSpace(renderedName), "."))
		if name == "" {
			continue
		}
		if name == "@" {
			if rootDomain != "" {
				name = rootDomain
			}
		} else if rootDomain != "" && name != rootDomain && !strings.HasSuffix(name, "."+rootDomain) {
			name = name + "." + rootDomain
		}

		renderedVal := strings.TrimSpace(apps.InterpolateVariables(rule.Value, globalVars, evalVars))
		if renderedVal == "" {
			continue
		}

		renderedTTL := strings.TrimSpace(apps.InterpolateVariables(rule.TTL, globalVars, evalVars))
		ttl := 1
		if renderedTTL != "" && !strings.EqualFold(renderedTTL, "auto") {
			parsedTTL, err := strconv.Atoi(renderedTTL)
			if err != nil || parsedTTL < 1 {
				errs = append(errs, fmt.Sprintf("custom_records[%d] (%s) for node %s: invalid TTL '%s'", i, name, node.Name, renderedTTL))
				continue
			}
			ttl = parsedTTL
		}

		proxied := rule.Proxied && (recType == "A" || recType == "AAAA" || recType == "CNAME")
		rec := DNSRecord{
			Type:    recType,
			Name:    name,
			TTL:     ttl,
			Proxied: proxied,
			Comment: easy42RecordComment,
		}

		switch recType {
		case "A":
			parsed := net.ParseIP(CleanIP(renderedVal))
			if parsed == nil || parsed.To4() == nil {
				errs = append(errs, fmt.Sprintf("custom_records[%d] (%s) for node %s: invalid IPv4 address '%s'", i, name, node.Name, renderedVal))
				continue
			}
			rec.Content = parsed.String()
		case "AAAA":
			parsed := net.ParseIP(CleanIP(renderedVal))
			if parsed == nil || parsed.To4() != nil {
				errs = append(errs, fmt.Sprintf("custom_records[%d] (%s) for node %s: invalid IPv6 address '%s'", i, name, node.Name, renderedVal))
				continue
			}
			rec.Content = parsed.String()
		case "CNAME":
			cname := strings.ToLower(strings.Trim(renderedVal, "."))
			if cname == "" {
				continue
			}
			rec.Content = cname
		case "TXT":
			rec.Content = renderedVal
		case "SRV":
			data, err := parseSRVValue(renderedVal)
			if err != nil {
				errs = append(errs, fmt.Sprintf("custom_records[%d] (%s) for node %s: %v", i, name, node.Name, err))
				continue
			}
			rec.Data = data
		default:
			errs = append(errs, fmt.Sprintf("custom_records[%d] (%s): unsupported record type '%s'", i, name, recType))
			continue
		}

		records = append(records, rec)
	}

	return records, errs
}

type dnsRecordKey struct {
	Type string
	Name string
}

func appendUniqueRecord(list []DNSRecord, rec DNSRecord) []DNSRecord {
	for _, existing := range list {
		if recordsMatch(existing, rec) {
			return list
		}
	}
	return append(list, rec)
}

func reconcileRecordGroup(ctx context.Context, client *Client, expectedList, existingList []DNSRecord, result *SyncResult) {
	matchedExisting := make([]bool, len(existingList))
	matchedExpected := make([]bool, len(expectedList))

	// First pass: keep exact matches
	for ei, exp := range expectedList {
		for xi, ex := range existingList {
			if matchedExisting[xi] {
				continue
			}
			if recordsMatch(ex, exp) {
				matchedExisting[xi] = true
				matchedExpected[ei] = true
				if result != nil {
					result.Ignored++
				}
				break
			}
		}
	}

	// Second pass: update unmatched existing records in-place to match unmatched expected records
	for ei, exp := range expectedList {
		if matchedExpected[ei] {
			continue
		}
		updated := false
		for xi, ex := range existingList {
			if matchedExisting[xi] {
				continue
			}
			matchedExisting[xi] = true
			matchedExpected[ei] = true
			if _, err := client.UpdateRecord(ctx, ex.ID, exp); err != nil {
				if result != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to update %s %s: %v", exp.Type, exp.Name, err))
				}
			} else if result != nil {
				result.Updated++
			}
			updated = true
			break
		}
		if !updated {
			if _, err := client.CreateRecord(ctx, exp); err != nil {
				if result != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to create %s %s: %v", exp.Type, exp.Name, err))
				}
			} else if result != nil {
				result.Created++
			}
		}
	}

	// Third pass: delete leftover existing records for this (Type, Name)
	for xi, ex := range existingList {
		if matchedExisting[xi] {
			continue
		}
		if err := client.DeleteRecord(ctx, ex.ID); err != nil {
			if result != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to delete duplicate %s: %v", ex.Name, err))
			}
		} else if result != nil {
			result.Deleted++
		}
	}
}

// SyncNode updates DNS records for a single node (best-effort)
func SyncNode(ctx context.Context, client *Client, node config.Node, oldName string, dnsCfg config.DNSConfig, globalVars ...map[string]string) error {
	var gVars map[string]string
	if len(globalVars) > 0 {
		gVars = globalVars[0]
	}
	return SyncNodeWithSSHKeys(ctx, client, node, oldName, dnsCfg, gVars, nil)
}

// SyncNodeWithSSHKeys updates DNS records for a single node including SSHFP records derived from agent-collected host keys
func SyncNodeWithSSHKeys(ctx context.Context, client *Client, node config.Node, oldName string, dnsCfg config.DNSConfig, globalVars map[string]string, nodeKeys []config.SSHPublicKey) error {
	if client == nil || !dnsCfg.IsConfigured() {
		return nil
	}

	baseDomain := NormalizeDomain(dnsCfg.BaseDomain)
	if baseDomain == "" {
		return nil
	}

	// If node was renamed, clean up old records for oldName
	if oldName != "" && !strings.EqualFold(oldName, node.Name) {
		oldClean := strings.ToLower(strings.TrimSpace(oldName))
		_ = DeleteNodeRecords(ctx, client, oldClean, dnsCfg)
	}

	nodeName := strings.ToLower(strings.TrimSpace(node.Name))
	if nodeName == "" {
		return nil
	}

	expectedMap := make(map[dnsRecordKey][]DNSRecord)
	for _, r := range ExpectedRecordsForNode(node, baseDomain, dnsCfg.PublishIPv6OwnName) {
		key := dnsRecordKey{Type: strings.ToUpper(r.Type), Name: strings.ToLower(r.Name)}
		expectedMap[key] = appendUniqueRecord(expectedMap[key], r)
	}

	for _, r := range ExpectedSSHFPRecordsForNode(node, dnsCfg.SSHFPRules, baseDomain, nodeKeys) {
		key := dnsRecordKey{Type: strings.ToUpper(r.Type), Name: strings.ToLower(r.Name)}
		expectedMap[key] = appendUniqueRecord(expectedMap[key], r)
	}

	if len(dnsCfg.CustomRecords) > 0 {
		rootDomain, err := client.GetZoneName(ctx)
		if err != nil {
			return fmt.Errorf("failed to resolve Cloudflare zone domain: %w", err)
		}
		customRecs, _ := ExpectedCustomRecordsForNode(node, dnsCfg.CustomRecords, baseDomain, rootDomain, globalVars)
		customMap := make(map[dnsRecordKey][]DNSRecord)
		for _, r := range customRecs {
			key := dnsRecordKey{Type: strings.ToUpper(r.Type), Name: strings.ToLower(r.Name)}
			customMap[key] = appendUniqueRecord(customMap[key], r)
		}
		for key, recs := range customMap {
			expectedMap[key] = recs
		}
	}

	// Fetch existing records for this node's built-in hostnames and any custom record names
	builtinNames := map[string]bool{
		fmt.Sprintf("%s.%s", nodeName, baseDomain):   true,
		fmt.Sprintf("*.%s.%s", nodeName, baseDomain): true,
		fmt.Sprintf("%s6.%s", nodeName, baseDomain):  true,
	}
	namesToQuery := make(map[string]bool, len(builtinNames)+len(expectedMap))
	for n := range builtinNames {
		namesToQuery[n] = true
	}
	for key := range expectedMap {
		namesToQuery[key.Name] = true
	}

	var existing []DNSRecord
	for n := range namesToQuery {
		recs, err := client.ListRecords(ctx, n)
		if err != nil {
			return fmt.Errorf("failed to query records for %s: %w", n, err)
		}
		existing = append(existing, recs...)
	}

	existingGrouped := make(map[dnsRecordKey][]DNSRecord)
	for _, r := range existing {
		key := dnsRecordKey{Type: strings.ToUpper(r.Type), Name: strings.ToLower(strings.Trim(r.Name, "."))}
		existingGrouped[key] = append(existingGrouped[key], r)
	}

	// Reconcile expected records
	for key, expectedList := range expectedMap {
		if builtinNames[key.Name] {
			reconcileRecordGroup(ctx, client, expectedList, existingGrouped[key], nil)
		} else {
			// For custom names outside built-in hostnames, ensure this node's expected records exist
			// without deleting records that may belong to other nodes sharing the same name.
			existingList := existingGrouped[key]
			for _, exp := range expectedList {
				found := false
				for _, ex := range existingList {
					if recordsMatch(ex, exp) {
						found = true
						break
					}
				}
				if !found {
					_, _ = client.CreateRecord(ctx, exp)
				}
			}
		}
	}

	// Delete any existing A or AAAA records for this node's built-in hostnames that are no longer expected
	for key, matches := range existingGrouped {
		if !builtinNames[key.Name] || (key.Type != "A" && key.Type != "AAAA") {
			continue
		}
		if _, isExpected := expectedMap[key]; !isExpected {
			for _, rec := range matches {
				_ = client.DeleteRecord(ctx, rec.ID)
			}
		}
	}

	return nil
}

// DeleteNodeRecords deletes all DNS records for a given node name
func DeleteNodeRecords(ctx context.Context, client *Client, nodeName string, dnsCfg config.DNSConfig) error {
	if client == nil || !dnsCfg.IsConfigured() {
		return nil
	}
	baseDomain := NormalizeDomain(dnsCfg.BaseDomain)
	if baseDomain == "" {
		return nil
	}
	nodeName = strings.ToLower(strings.TrimSpace(nodeName))
	if nodeName == "" {
		return nil
	}

	names := []string{
		fmt.Sprintf("%s.%s", nodeName, baseDomain),
		fmt.Sprintf("*.%s.%s", nodeName, baseDomain),
		fmt.Sprintf("%s6.%s", nodeName, baseDomain),
	}

	var errs []string
	for _, n := range names {
		recs, err := client.ListRecords(ctx, n)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, r := range recs {
			if r.Type == "A" || r.Type == "AAAA" || r.Type == "SSHFP" {
				if err := client.DeleteRecord(ctx, r.ID); err != nil {
					errs = append(errs, err.Error())
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors deleting records: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Sync synchronizes all easy42 nodes to Cloudflare DNS
func Sync(ctx context.Context, client *Client, nodes []config.Node, dnsCfg config.DNSConfig, force bool, globalVars ...map[string]string) (*SyncResult, error) {
	var gVars map[string]string
	if len(globalVars) > 0 {
		gVars = globalVars[0]
	}
	return SyncWithSSHKeys(ctx, client, nodes, dnsCfg, force, gVars, nil)
}

// SyncWithSSHKeys synchronizes all easy42 nodes to Cloudflare DNS, including SSHFP records from agent-collected host keys
func SyncWithSSHKeys(ctx context.Context, client *Client, nodes []config.Node, dnsCfg config.DNSConfig, force bool, globalVars map[string]string, hostKeysByNode map[string][]config.SSHPublicKey) (*SyncResult, error) {
	if client == nil {
		return nil, fmt.Errorf("dns client is nil")
	}
	if !dnsCfg.IsConfigured() {
		return nil, fmt.Errorf("cloudflare DNS is not configured")
	}

	baseDomain := NormalizeDomain(dnsCfg.BaseDomain)
	if baseDomain == "" {
		return nil, fmt.Errorf("base domain cannot be empty")
	}

	result := &SyncResult{}

	var rootDomain string
	if len(dnsCfg.CustomRecords) > 0 {
		var err error
		rootDomain, err = client.GetZoneName(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve Cloudflare zone domain: %w", err)
		}
	}

	// 1. Fetch all records for the zone
	allRecords, err := client.ListRecords(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zone DNS records: %w", err)
	}

	// 2. Build map of expected records from easy42 nodes (built-in + SSHFP + custom, where custom overrides)
	expectedMap := make(map[dnsRecordKey][]DNSRecord)
	customMap := make(map[dnsRecordKey][]DNSRecord)
	validNodeNames := make(map[string]bool)

	for _, node := range nodes {
		cleanName := strings.ToLower(strings.TrimSpace(node.Name))
		if node.HasRoutableMainIP() && cleanName != "" {
			validNodeNames[cleanName] = true
			for _, rec := range ExpectedRecordsForNode(node, baseDomain, dnsCfg.PublishIPv6OwnName) {
				key := dnsRecordKey{Type: strings.ToUpper(rec.Type), Name: strings.ToLower(rec.Name)}
				expectedMap[key] = appendUniqueRecord(expectedMap[key], rec)
			}
		}

		if len(dnsCfg.SSHFPRules) > 0 && cleanName != "" && len(hostKeysByNode) > 0 {
			nodeKeys := hostKeysByNode[cleanName]
			if len(nodeKeys) == 0 {
				nodeKeys = hostKeysByNode[node.Name]
			}
			for _, rec := range ExpectedSSHFPRecordsForNode(node, dnsCfg.SSHFPRules, baseDomain, nodeKeys) {
				key := dnsRecordKey{Type: strings.ToUpper(rec.Type), Name: strings.ToLower(rec.Name)}
				expectedMap[key] = appendUniqueRecord(expectedMap[key], rec)
			}
		}

		if len(dnsCfg.CustomRecords) > 0 {
			customRecs, customErrs := ExpectedCustomRecordsForNode(node, dnsCfg.CustomRecords, baseDomain, rootDomain, globalVars)
			result.Errors = append(result.Errors, customErrs...)
			for _, rec := range customRecs {
				key := dnsRecordKey{Type: strings.ToUpper(rec.Type), Name: strings.ToLower(rec.Name)}
				customMap[key] = appendUniqueRecord(customMap[key], rec)
			}
		}
	}

	// Custom records have higher priority and override built-in records for the same (Type, Name)
	for key, recs := range customMap {
		expectedMap[key] = recs
	}

	// 3. Index existing records by (type, name)
	existingMap := make(map[dnsRecordKey][]DNSRecord)
	for _, rec := range allRecords {
		key := dnsRecordKey{
			Type: strings.ToUpper(rec.Type),
			Name: strings.ToLower(strings.Trim(rec.Name, ".")),
		}
		existingMap[key] = append(existingMap[key], rec)
	}

	// 4. Create or update expected records
	for key, expectedList := range expectedMap {
		reconcileRecordGroup(ctx, client, expectedList, existingMap[key], result)
	}

	// 5. In non-force sync, clean up stale AAAA records for existing nodes when toggle changed
	if !force {
		for cleanNodeName := range validNodeNames {
			nodeNameDomain := fmt.Sprintf("%s.%s", cleanNodeName, baseDomain)
			nodeWildDomain := fmt.Sprintf("*.%s.%s", cleanNodeName, baseDomain)
			nodeName6Domain := fmt.Sprintf("%s6.%s", cleanNodeName, baseDomain)

			if dnsCfg.PublishIPv6OwnName {
				// Old <name>.<domain> and *.<name>.<domain> AAAA records should be removed (unless overridden by custom rule)
				for _, staleName := range []string{nodeNameDomain, nodeWildDomain} {
					staleKey := dnsRecordKey{Type: "AAAA", Name: staleName}
					if _, expected := expectedMap[staleKey]; expected {
						continue
					}
					if recs, ok := existingMap[staleKey]; ok {
						for _, r := range recs {
							if err := client.DeleteRecord(ctx, r.ID); err != nil {
								result.Errors = append(result.Errors, fmt.Sprintf("failed to delete stale AAAA %s: %v", r.Name, err))
							} else {
								result.Deleted++
							}
						}
					}
				}
			} else {
				// Old <name>6.<domain> AAAA records should be removed (unless overridden by custom rule)
				staleKey := dnsRecordKey{Type: "AAAA", Name: nodeName6Domain}
				if _, expected := expectedMap[staleKey]; !expected {
					if recs, ok := existingMap[staleKey]; ok {
						for _, r := range recs {
							if err := client.DeleteRecord(ctx, r.ID); err != nil {
								result.Errors = append(result.Errors, fmt.Sprintf("failed to delete stale AAAA %s: %v", r.Name, err))
							} else {
								result.Deleted++
							}
						}
					}
				}
			}
		}
	}

	// 6. In Force Sync: delete all existing "*.<domain>" records where the corresponding name node doesn't exist
	if force {
		domainSuffix := "." + baseDomain
		for _, rec := range allRecords {
			recName := strings.ToLower(strings.Trim(rec.Name, "."))
			recType := strings.ToUpper(rec.Type)

			// Only process records that are subdomains of baseDomain (i.e. *.<baseDomain>)
			if !strings.HasSuffix(recName, domainSuffix) || recName == baseDomain {
				continue
			}

			// We manage A, AAAA, CNAME, and SSHFP records under *.<baseDomain>
			if recType != "A" && recType != "AAAA" && recType != "CNAME" && recType != "SSHFP" {
				continue
			}

			// Never delete records that are expected (e.g. custom records)
			if _, expected := expectedMap[dnsRecordKey{Type: recType, Name: recName}]; expected {
				continue
			}

			// Extract subdomain (everything before .baseDomain)
			sub := strings.TrimSuffix(recName, domainSuffix)
			if sub == "" {
				continue
			}

			// Check if this record corresponds to an existing node
			correspondsToNode := false

			checkSub := sub
			isWildcard := false
			if strings.HasPrefix(checkSub, "*.") {
				isWildcard = true
				checkSub = strings.TrimPrefix(checkSub, "*.")
			}

			if validNodeNames[checkSub] {
				// Sub matches node name exactly
				if recType == "A" {
					correspondsToNode = true
				} else if recType == "AAAA" && !dnsCfg.PublishIPv6OwnName {
					correspondsToNode = true
				}
			} else if !isWildcard && dnsCfg.PublishIPv6OwnName && strings.HasSuffix(sub, "6") {
				baseNode := strings.TrimSuffix(sub, "6")
				if validNodeNames[baseNode] && recType == "AAAA" {
					correspondsToNode = true
				}
			}

			if !correspondsToNode {
				// Delete record: node doesn't exist in easy42 network
				if err := client.DeleteRecord(ctx, rec.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to delete orphaned record %s (%s): %v", rec.Name, rec.Type, err))
				} else {
					result.Deleted++
				}
			}
		}
	}

	if len(result.Errors) > 0 {
		result.Message = fmt.Sprintf("Completed with %d error(s)", len(result.Errors))
	} else {
		result.Message = fmt.Sprintf("Successfully synced DNS: %d created, %d updated, %d deleted, %d unchanged",
			result.Created, result.Updated, result.Deleted, result.Ignored)
	}

	return result, nil
}
