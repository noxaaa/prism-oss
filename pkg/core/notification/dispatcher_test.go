package notification

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestValidateWebhookDestinationRejectsInternalTargets(t *testing.T) {
	resolver := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.0.0.10")}, nil
	}

	for _, rawURL := range []string{
		"http://127.0.0.1/hook",
		"https://localhost/hook",
		"https://[::1]/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://internal.example/hook",
		"ftp://hooks.example.com/hook",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if err := validateWebhookDestination(context.Background(), rawURL, resolver); err == nil {
				t.Fatalf("expected %s to be rejected", rawURL)
			}
		})
	}
}

func TestValidateWebhookDestinationAllowsPublicTargets(t *testing.T) {
	resolver := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}

	if err := validateWebhookDestination(context.Background(), "https://hooks.example.com/prism", resolver); err != nil {
		t.Fatalf("expected public webhook URL to be accepted: %v", err)
	}
}

func TestWebhookDestinationErrorsDoNotExposeSecret(t *testing.T) {
	err := sendWebhook(context.Background(), `{"url":"http://127.0.0.1/hook"}`, "top-secret", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected localhost webhook to be rejected")
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("webhook validation error exposed secret: %v", err)
	}
}

func TestWebhookRedirectsAreRevalidated(t *testing.T) {
	resolver := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}
	client := newWebhookHTTPClient(resolver)
	redirectRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1/metadata", nil)
	if err != nil {
		t.Fatalf("build redirect request: %v", err)
	}
	originalRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://hooks.example.com/prism", nil)
	if err != nil {
		t.Fatalf("build original request: %v", err)
	}

	err = client.CheckRedirect(redirectRequest, []*http.Request{originalRequest})
	if err == nil {
		t.Fatalf("expected redirect to internal address to be rejected")
	}
}

func TestWebhookDialAddressIsValidated(t *testing.T) {
	calls := 0
	resolver := func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}

	err := validateWebhookDestination(context.Background(), "https://hooks.example.com/prism", resolver)
	if err != nil {
		t.Fatalf("preflight validation should accept first public resolution: %v", err)
	}
	_, err = dialAllowedWebhookAddress(context.Background(), &net.Dialer{Timeout: time.Millisecond}, resolver, "tcp", "hooks.example.com:443")
	if err == nil {
		t.Fatalf("expected dial-time private resolution to be rejected")
	}
}

func TestSMTPDialAddressIsValidated(t *testing.T) {
	resolver := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	}

	_, err := dialAllowedSMTPAddress(context.Background(), resolver, "tcp", "smtp.example.com", "587")
	if err == nil {
		t.Fatalf("expected SMTP private metadata address to be rejected")
	}
}

func TestSendEmailRejectsSubjectHeaderInjection(t *testing.T) {
	err := sendEmail(context.Background(), `{"smtp_host":"smtp.example.com","from":"from@example.com","to":["to@example.com"],"subject":"hello\r\nBcc: attacker@example.com"}`, "", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected CRLF subject to be rejected")
	}
	if !strings.Contains(err.Error(), "subject") {
		t.Fatalf("expected subject validation error, got %v", err)
	}
}

func TestSendEmailHonorsTimeout(t *testing.T) {
	done := make(chan struct{})

	previousTimeout := notificationSendTimeout
	notificationSendTimeout = 50 * time.Millisecond
	defer func() { notificationSendTimeout = previousTimeout }()
	previousDial := smtpDialContext
	smtpDialContext = func(context.Context, string, string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer close(done)
			defer func() { _ = serverConn.Close() }()
			time.Sleep(500 * time.Millisecond)
		}()
		return clientConn, nil
	}
	defer func() { smtpDialContext = previousDial }()
	previousResolver := smtpResolver
	smtpResolver = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}
	defer func() { smtpResolver = previousResolver }()

	startedAt := time.Now()
	err := sendEmail(context.Background(), `{"smtp_host":"smtp.example.com","smtp_port":587,"from":"from@example.com","to":["to@example.com"]}`, "", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected hanging SMTP server to time out")
	}
	if elapsed := time.Since(startedAt); elapsed > 400*time.Millisecond {
		t.Fatalf("email send did not honor timeout quickly enough: %s", elapsed)
	}
	<-done
}
