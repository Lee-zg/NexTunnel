package relay

import (
	"flag"
	"fmt"
	"time"

	"github.com/nextunnel/pkg/tlsutil"
)

// Config holds the relay server configuration.
type Config struct {
	BindAddr            string
	ControlPort         int
	QUICPort            int
	AuthToken           string
	RequireAuth         bool // When true, refuse to start without AuthToken
	AdminListenAddr     string
	AdminToken          string
	HeartbeatTimeout    time.Duration
	MaxProxiesPerClient int
	WorkConnTimeout     time.Duration
	TLSEnabled          bool
	TLS                 tlsutil.TLSConfig
	PublicHTTPListen    string
	PublicHTTPSListen   string
	DomainSuffix        string
	PublicTLSMode       string // off, file, acme
	PublicTLSCert       string
	PublicTLSKey        string
	ACMEEmail           string
	RequestLogRetention int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BindAddr:            "0.0.0.0",
		ControlPort:         7000,
		QUICPort:            7443,
		HeartbeatTimeout:    90 * time.Second,
		MaxProxiesPerClient: 100,
		WorkConnTimeout:     30 * time.Second,
		PublicTLSMode:       "off",
		RequestLogRetention: 1000,
	}
}

// ParseFlags parses CLI flags into a Config.
func ParseFlags(fs *flag.FlagSet) *Config {
	cfg := DefaultConfig()
	fs.StringVar(&cfg.BindAddr, "bind", cfg.BindAddr, "bind address")
	fs.IntVar(&cfg.ControlPort, "control-port", cfg.ControlPort, "control port for client connections")
	fs.StringVar(&cfg.AuthToken, "auth-token", cfg.AuthToken, "shared auth token for relay clients (required for non-local deployments)")
	fs.BoolVar(&cfg.RequireAuth, "require-auth", cfg.RequireAuth, "require auth token (auto-enabled for non-localhost bind)")
	fs.StringVar(&cfg.AdminListenAddr, "admin-listen", cfg.AdminListenAddr, "optional admin HTTP listen address for Dashboard integration")
	fs.StringVar(&cfg.AdminToken, "admin-token", cfg.AdminToken, "Bearer token required by the admin HTTP API")
	fs.DurationVar(&cfg.HeartbeatTimeout, "heartbeat-timeout", cfg.HeartbeatTimeout, "heartbeat timeout")
	fs.IntVar(&cfg.MaxProxiesPerClient, "max-proxies", cfg.MaxProxiesPerClient, "max proxies per client")
	fs.DurationVar(&cfg.WorkConnTimeout, "work-conn-timeout", cfg.WorkConnTimeout, "timeout waiting for work connection")
	fs.IntVar(&cfg.QUICPort, "quic-port", cfg.QUICPort, "QUIC transport port")
	fs.StringVar(&cfg.TLS.CACert, "tls-ca", "", "CA certificate PEM for mTLS (enables TLS when set)")
	fs.StringVar(&cfg.TLS.Cert, "tls-cert", "", "server certificate PEM for mTLS")
	fs.StringVar(&cfg.TLS.Key, "tls-key", "", "server private key PEM for mTLS")
	fs.StringVar(&cfg.PublicHTTPListen, "public-http-listen", "", "optional Public Endpoint HTTP gateway listen address, for example 0.0.0.0:80")
	fs.StringVar(&cfg.PublicHTTPSListen, "public-https-listen", "", "optional Public Endpoint HTTPS gateway listen address, for example 0.0.0.0:443")
	fs.StringVar(&cfg.DomainSuffix, "domain-suffix", "", "domain suffix used when clients publish by subdomain")
	fs.StringVar(&cfg.PublicTLSMode, "tls-mode", cfg.PublicTLSMode, "Public Endpoint TLS mode: off, file, or acme")
	fs.StringVar(&cfg.PublicTLSCert, "public-tls-cert", "", "certificate file for Public Endpoint HTTPS when tls-mode=file")
	fs.StringVar(&cfg.PublicTLSKey, "public-tls-key", "", "private key file for Public Endpoint HTTPS when tls-mode=file")
	fs.StringVar(&cfg.ACMEEmail, "acme-email", "", "ACME account email reserved for tls-mode=acme")
	fs.IntVar(&cfg.RequestLogRetention, "request-log-retention", cfg.RequestLogRetention, "number of Public Endpoint HTTP request logs kept in memory")
	return cfg
}

// Validate checks the configuration for security and correctness.
// Returns an error if the configuration is invalid.
func (c *Config) Validate() error {
	// Auto-enable RequireAuth for non-localhost deployments
	if !c.RequireAuth && c.BindAddr != "127.0.0.1" && c.BindAddr != "localhost" && c.BindAddr != "::1" {
		c.RequireAuth = true
	}
	if c.RequireAuth && c.AuthToken == "" {
		return fmt.Errorf("security: auth-token is required for non-local bind address %q; use -auth-token or bind to 127.0.0.1 for development", c.BindAddr)
	}
	if c.AdminListenAddr != "" && c.AdminToken == "" {
		return fmt.Errorf("security: admin-token is required when admin-listen is configured")
	}
	if c.ControlPort <= 0 || c.ControlPort > 65535 {
		return fmt.Errorf("invalid control port: %d", c.ControlPort)
	}
	switch c.PublicTLSMode {
	case "", "off":
		c.PublicTLSMode = "off"
		if c.PublicHTTPSListen != "" {
			return fmt.Errorf("public endpoint https listen requires tls-mode=file; use public-http-listen or an external TLS reverse proxy when tls-mode=off")
		}
	case "file":
		if c.PublicHTTPSListen != "" && (c.PublicTLSCert == "" || c.PublicTLSKey == "") {
			return fmt.Errorf("public endpoint tls-mode=file requires public-tls-cert and public-tls-key")
		}
	case "acme":
		return fmt.Errorf("public endpoint tls-mode=acme is reserved but not implemented; use tls-mode=file or external reverse proxy")
	default:
		return fmt.Errorf("unsupported public endpoint tls-mode: %s", c.PublicTLSMode)
	}
	if c.RequestLogRetention < 0 {
		return fmt.Errorf("request-log-retention must be >= 0")
	}
	// Auto-enable TLS when all cert paths are provided
	if c.TLS.Enabled() {
		c.TLSEnabled = true
	}
	return nil
}
