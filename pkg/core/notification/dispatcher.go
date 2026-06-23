package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var notificationSendTimeout = 10 * time.Second
var smtpDialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: notificationSendTimeout}).DialContext(ctx, network, address)
}
var smtpResolver = net.DefaultResolver.LookupNetIP

func Send(ctx context.Context, channelType string, configJSON string, secret string, payload []byte) error {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case "WEBHOOK":
		return sendWebhook(ctx, configJSON, secret, payload)
	case "EMAIL":
		return sendEmail(ctx, configJSON, secret, payload)
	default:
		return fmt.Errorf("unsupported notification channel type")
	}
}

func sendWebhook(ctx context.Context, configJSON string, secret string, payload []byte) error {
	var config struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return err
	}
	if strings.TrimSpace(config.URL) == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if err := validateWebhookDestination(ctx, config.URL, net.DefaultResolver.LookupNetIP); err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(config.Method))
	if method == "" {
		method = http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, config.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(secret) != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp))
		mac.Write([]byte("."))
		mac.Write(payload)
		request.Header.Set("X-Prism-Timestamp", timestamp)
		request.Header.Set("X-Prism-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	for key, value := range config.Headers {
		request.Header.Set(key, value)
	}
	response, err := newWebhookHTTPClient(net.DefaultResolver.LookupNetIP).Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

type webhookResolver func(context.Context, string, string) ([]netip.Addr, error)

func newWebhookHTTPClient(resolver webhookResolver) *http.Client {
	dialer := &net.Dialer{Timeout: notificationSendTimeout}
	return &http.Client{
		Timeout: notificationSendTimeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
				return dialAllowedWebhookAddress(ctx, dialer, resolver, network, address)
			},
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return validateWebhookDestination(request.Context(), request.URL.String(), resolver)
		},
	}
}

func dialAllowedWebhookAddress(ctx context.Context, dialer *net.Dialer, resolver webhookResolver, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if parsed, err := netip.ParseAddr(host); err == nil {
		if !isAllowedWebhookAddress(parsed) {
			return nil, fmt.Errorf("webhook dial address is not allowed")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(parsed.String(), port))
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("webhook dial host is not allowed")
	}
	addresses, err := resolver(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("webhook dial host did not resolve")
	}
	var lastErr error
	for _, candidate := range addresses {
		if !isAllowedWebhookAddress(candidate) {
			return nil, fmt.Errorf("webhook dial address is not allowed")
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("webhook dial host did not resolve to a usable address")
}

func validateWebhookDestination(ctx context.Context, rawURL string, resolver webhookResolver) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("webhook URL scheme must be http or https")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("webhook URL host is required")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("webhook URL host is not allowed")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if !isAllowedWebhookAddress(address) {
			return fmt.Errorf("webhook URL address is not allowed")
		}
		return nil
	}
	addresses, err := resolver(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return fmt.Errorf("webhook URL host did not resolve")
	}
	for _, address := range addresses {
		if !isAllowedWebhookAddress(address) {
			return fmt.Errorf("webhook URL resolved to an address that is not allowed")
		}
	}
	return nil
}

func isAllowedWebhookAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() ||
		address.IsUnspecified() {
		return false
	}
	return !isCarrierGradeNATAddress(address)
}

func isCarrierGradeNATAddress(address netip.Addr) bool {
	if !address.Is4() {
		return false
	}
	prefix := netip.MustParsePrefix("100.64.0.0/10")
	return prefix.Contains(address)
}

func sendEmail(ctx context.Context, configJSON string, password string, payload []byte) error {
	var config struct {
		SMTPHost string   `json:"smtp_host"`
		SMTPPort int      `json:"smtp_port"`
		Username string   `json:"username"`
		From     string   `json:"from"`
		To       []string `json:"to"`
		Subject  string   `json:"subject"`
	}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return err
	}
	if config.SMTPHost == "" || config.From == "" || len(config.To) == 0 {
		return fmt.Errorf("email SMTP host, sender, and recipients are required")
	}
	port := config.SMTPPort
	if port == 0 {
		port = 587
	}
	subject := config.Subject
	if subject == "" {
		subject = "Prism DNS policy notification"
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("email subject must not contain CR or LF characters")
	}
	message := []byte("Subject: " + subject + "\r\nContent-Type: application/json\r\n\r\n" + string(payload))
	ctx, cancel := context.WithTimeout(ctx, notificationSendTimeout)
	defer cancel()
	connection, err := dialAllowedSMTPAddress(ctx, smtpResolver, "tcp", config.SMTPHost, strconv.Itoa(port))
	if err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = connection.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(connection, config.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return err
	}
	defer func() { _ = client.Close() }()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if config.Username != "" || password != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", config.Username, password, config.SMTPHost)); err != nil {
			return err
		}
	}
	if err := client.Mail(config.From); err != nil {
		return err
	}
	for _, recipient := range config.To {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func dialAllowedSMTPAddress(ctx context.Context, resolver webhookResolver, network string, host string, port string) (net.Conn, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if parsed, err := netip.ParseAddr(host); err == nil {
		if !isAllowedWebhookAddress(parsed) {
			return nil, fmt.Errorf("smtp dial address is not allowed")
		}
		return smtpDialContext(ctx, network, net.JoinHostPort(parsed.String(), port))
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("smtp dial host is not allowed")
	}
	addresses, err := resolver(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("smtp dial host did not resolve")
	}
	var lastErr error
	for _, candidate := range addresses {
		if !isAllowedWebhookAddress(candidate) {
			return nil, fmt.Errorf("smtp dial address is not allowed")
		}
		conn, err := smtpDialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("smtp dial host did not resolve to a usable address")
}
