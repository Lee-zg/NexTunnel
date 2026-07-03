package relay

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextunnel/pkg/tlsutil"
)

func TestConfig_TLSValidation(t *testing.T) {
	dir := t.TempDir()
	caCert, caX509, err := tlsutil.GenerateSelfSignedCA()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA: %v", err)
	}
	caPath := filepath.Join(dir, "ca.pem")
	if err := tlsutil.WritePEMFiles(caCert, caPath, filepath.Join(dir, "ca-key.pem")); err != nil {
		t.Fatal(err)
	}
	serverCert, err := tlsutil.GenerateSignedCert(caX509, caCert.PrivateKey, "relay", true)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	if err := tlsutil.WritePEMFiles(serverCert, certPath, keyPath); err != nil {
		t.Fatal(err)
	}

	t.Run("TLS auto-enabled with valid paths", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.BindAddr = "127.0.0.1"
		cfg.AuthToken = "test"
		cfg.TLS = tlsutil.TLSConfig{CACert: caPath, Cert: certPath, Key: keyPath}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if !cfg.TLSEnabled {
			t.Error("TLSEnabled should be true when TLS paths are provided")
		}
	})

	t.Run("TLS disabled without paths", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.BindAddr = "127.0.0.1"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if cfg.TLSEnabled {
			t.Error("TLSEnabled should be false without TLS paths")
		}
	})
}

func TestServer_RunWithTLS(t *testing.T) {
	dir := t.TempDir()

	caCert, caX509, err := tlsutil.GenerateSelfSignedCA()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA: %v", err)
	}
	caPath := filepath.Join(dir, "ca.pem")
	if err := tlsutil.WritePEMFiles(caCert, caPath, filepath.Join(dir, "ca-key.pem")); err != nil {
		t.Fatal(err)
	}
	serverCert, err := tlsutil.GenerateSignedCert(caX509, caCert.PrivateKey, "relay", true)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	if err := tlsutil.WritePEMFiles(serverCert, certPath, keyPath); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0 // random port
	cfg.QUICPort = 0    // disable QUIC for this test
	cfg.AuthToken = "test-token"
	cfg.TLSEnabled = true
	cfg.TLS = tlsutil.TLSConfig{CACert: caPath, Cert: certPath, Key: keyPath}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	srv := NewServer(cfg, logger)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run with TLS: %v", err)
	}
	defer srv.Shutdown(context.Background())

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("server addr is nil")
	}
	t.Logf("relay TLS server listening on %s", addr.String())
}

func TestServer_RunWithoutTLS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BindAddr = "127.0.0.1"
	cfg.ControlPort = 0
	cfg.QUICPort = 0

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	srv := NewServer(cfg, logger)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run without TLS: %v", err)
	}
	defer srv.Shutdown(context.Background())

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("server addr is nil")
	}
}
