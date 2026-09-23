package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"easy42/internal/config"
)

const (
	DefaultCloudflareAPIURL = "https://api.cloudflare.com/client/v4"
	easy42RecordComment     = "easy42 managed"
)

// DNSRecord represents a Cloudflare DNS record
type DNSRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment,omitempty"`
	ZoneID   string `json:"zone_id,omitempty"`
	ZoneName string `json:"zone_name,omitempty"`
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

// CreateRecord creates a new DNS record
func (c *Client) CreateRecord(ctx context.Context, rec DNSRecord) (*DNSRecord, error) {
	endpoint := fmt.Sprintf("/zones/%s/dns_records", c.zoneID)
	payload := map[string]any{
		"type":    rec.Type,
		"name":    rec.Name,
		"content": rec.Content,
		"ttl":     1, // Automatic TTL
		"proxied": false,
		"comment": easy42RecordComment,
	}

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
	payload := map[string]any{
		"type":    rec.Type,
		"name":    rec.Name,
		"content": rec.Content,
		"ttl":     1,
		"proxied": false,
		"comment": easy42RecordComment,
	}

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
func ExpectedRecordsForNode(node config.Node, baseDomain string, publishIPv6OwnName bool) (aRec *DNSRecord, aaaaRec *DNSRecord) {
	nodeName := strings.ToLower(strings.TrimSpace(node.Name))
	if nodeName == "" {
		return nil, nil
	}
	baseDomain = NormalizeDomain(baseDomain)
	if baseDomain == "" {
		return nil, nil
	}

	ipv4 := CleanIP(node.IP)
	ipv6 := CleanIP(node.IP6)

	// Validate IPv4
	if ipv4 != "" {
		if parsed := net.ParseIP(ipv4); parsed != nil && parsed.To4() != nil {
			aRec = &DNSRecord{
				Type:    "A",
				Name:    fmt.Sprintf("%s.%s", nodeName, baseDomain),
				Content: parsed.String(),
				TTL:     1,
				Proxied: false,
				Comment: easy42RecordComment,
			}
		}
	}

	// Validate IPv6
	if ipv6 != "" {
		if parsed := net.ParseIP(ipv6); parsed != nil && parsed.To4() == nil {
			var recName string
			if publishIPv6OwnName {
				recName = fmt.Sprintf("%s6.%s", nodeName, baseDomain)
			} else {
				recName = fmt.Sprintf("%s.%s", nodeName, baseDomain)
			}
			aaaaRec = &DNSRecord{
				Type:    "AAAA",
				Name:    recName,
				Content: parsed.String(),
				TTL:     1,
				Proxied: false,
				Comment: easy42RecordComment,
			}
		}
	}

	return aRec, aaaaRec
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

// SyncNode updates DNS records for a single node (best-effort)
func SyncNode(ctx context.Context, client *Client, node config.Node, oldName string, dnsCfg config.DNSConfig) error {
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

	aExpected, aaaaExpected := ExpectedRecordsForNode(node, baseDomain, dnsCfg.PublishIPv6OwnName)

	// Fetch existing records for this node's potential hostnames
	namesToQuery := []string{
		fmt.Sprintf("%s.%s", nodeName, baseDomain),
		fmt.Sprintf("%s6.%s", nodeName, baseDomain),
	}

	var existing []DNSRecord
	for _, n := range namesToQuery {
		recs, err := client.ListRecords(ctx, n)
		if err != nil {
			return fmt.Errorf("failed to query records for %s: %w", n, err)
		}
		existing = append(existing, recs...)
	}

	// Reconcile A record
	syncSingleExpected(ctx, client, "A", aExpected, existing)

	// Reconcile AAAA record
	syncSingleExpected(ctx, client, "AAAA", aaaaExpected, existing)

	// Also delete any obsolete AAAA records (e.g. if toggle is ON, delete <name>.<domain> AAAA; if toggle is OFF, delete <name>6.<domain> AAAA)
	targetAAAAName := ""
	if aaaaExpected != nil {
		targetAAAAName = strings.ToLower(aaaaExpected.Name)
	}
	for _, rec := range existing {
		if rec.Type == "AAAA" && (targetAAAAName == "" || !strings.EqualFold(rec.Name, targetAAAAName)) {
			// This AAAA record is not the expected target for this node, delete it
			_ = client.DeleteRecord(ctx, rec.ID)
		}
	}

	return nil
}

func syncSingleExpected(ctx context.Context, client *Client, recType string, expected *DNSRecord, existing []DNSRecord) {
	var matching []DNSRecord
	for _, rec := range existing {
		if rec.Type == recType {
			if expected != nil && strings.EqualFold(rec.Name, expected.Name) {
				matching = append(matching, rec)
			}
		}
	}

	if expected == nil {
		// No record expected: delete any matching records
		for _, rec := range matching {
			_ = client.DeleteRecord(ctx, rec.ID)
		}
		return
	}

	if len(matching) == 0 {
		// Create record
		_, _ = client.CreateRecord(ctx, *expected)
		return
	}

	// One or more records already exist
	keep := matching[0]
	if !IPMatches(keep.Content, expected.Content) || keep.Proxied != expected.Proxied {
		_, _ = client.UpdateRecord(ctx, keep.ID, *expected)
	}

	// Delete any duplicate records
	for i := 1; i < len(matching); i++ {
		_ = client.DeleteRecord(ctx, matching[i].ID)
	}
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
			if r.Type == "A" || r.Type == "AAAA" {
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
func Sync(ctx context.Context, client *Client, nodes []config.Node, dnsCfg config.DNSConfig, force bool) (*SyncResult, error) {
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

	// 1. Fetch all records for the zone
	allRecords, err := client.ListRecords(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zone DNS records: %w", err)
	}

	// 2. Build map of expected records from easy42 nodes
	type recordKey struct {
		Type string
		Name string
	}

	expectedMap := make(map[recordKey]*DNSRecord)
	validNodeNames := make(map[string]bool)

	for _, node := range nodes {
		cleanName := strings.ToLower(strings.TrimSpace(node.Name))
		if cleanName == "" {
			continue
		}
		validNodeNames[cleanName] = true

		aRec, aaaaRec := ExpectedRecordsForNode(node, baseDomain, dnsCfg.PublishIPv6OwnName)
		if aRec != nil {
			expectedMap[recordKey{Type: aRec.Type, Name: strings.ToLower(aRec.Name)}] = aRec
		}
		if aaaaRec != nil {
			expectedMap[recordKey{Type: aaaaRec.Type, Name: strings.ToLower(aaaaRec.Name)}] = aaaaRec
		}
	}

	// 3. Index existing records by (type, name)
	existingMap := make(map[recordKey][]DNSRecord)
	for _, rec := range allRecords {
		key := recordKey{
			Type: strings.ToUpper(rec.Type),
			Name: strings.ToLower(strings.Trim(rec.Name, ".")),
		}
		existingMap[key] = append(existingMap[key], rec)
	}

	// 4. Create or update expected records
	for key, expected := range expectedMap {
		existingList := existingMap[key]
		if len(existingList) == 0 {
			// Create record
			_, err := client.CreateRecord(ctx, *expected)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create %s %s: %v", expected.Type, expected.Name, err))
			} else {
				result.Created++
			}
		} else {
			// Keep first record, check if matching
			primary := existingList[0]
			if IPMatches(primary.Content, expected.Content) && primary.Proxied == expected.Proxied {
				result.Ignored++
			} else {
				_, err := client.UpdateRecord(ctx, primary.ID, *expected)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to update %s %s: %v", expected.Type, expected.Name, err))
				} else {
					result.Updated++
				}
			}

			// Delete extra duplicate records with the same name and type
			for i := 1; i < len(existingList); i++ {
				if err := client.DeleteRecord(ctx, existingList[i].ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to delete duplicate %s: %v", existingList[i].Name, err))
				} else {
					result.Deleted++
				}
			}
		}
	}

	// 5. In non-force sync, clean up stale AAAA records for existing nodes when toggle changed
	if !force {
		for cleanNodeName := range validNodeNames {
			nodeNameDomain := fmt.Sprintf("%s.%s", cleanNodeName, baseDomain)
			nodeName6Domain := fmt.Sprintf("%s6.%s", cleanNodeName, baseDomain)

			if dnsCfg.PublishIPv6OwnName {
				// Old <name>.<domain> AAAA records should be removed
				if recs, ok := existingMap[recordKey{Type: "AAAA", Name: nodeNameDomain}]; ok {
					for _, r := range recs {
						if err := client.DeleteRecord(ctx, r.ID); err != nil {
							result.Errors = append(result.Errors, fmt.Sprintf("failed to delete stale AAAA %s: %v", r.Name, err))
						} else {
							result.Deleted++
						}
					}
				}
			} else {
				// Old <name>6.<domain> AAAA records should be removed
				if recs, ok := existingMap[recordKey{Type: "AAAA", Name: nodeName6Domain}]; ok {
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

			// We manage A, AAAA, and CNAME records under *.<baseDomain>
			if recType != "A" && recType != "AAAA" && recType != "CNAME" {
				continue
			}

			// Extract subdomain (everything before .baseDomain)
			sub := strings.TrimSuffix(recName, domainSuffix)
			if sub == "" {
				continue
			}

			// Check if this record corresponds to an existing node
			correspondsToNode := false

			if validNodeNames[sub] {
				// Sub matches node name exactly
				if recType == "A" {
					correspondsToNode = true
				} else if recType == "AAAA" && !dnsCfg.PublishIPv6OwnName {
					correspondsToNode = true
				}
			} else if dnsCfg.PublishIPv6OwnName && strings.HasSuffix(sub, "6") {
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
