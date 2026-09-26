// Package grpctls terminates TLS on the API server's gRPC listener.
//
// The rh-trex-ai framework registers --grpc-enable-tls, --grpc-tls-cert-file and
// --grpc-tls-key-file, but it only honours them when its shared TLS
// configuration fails to build. With the shared --enable-tls left off (turning
// it on would also serve the REST listener over HTTPS) those flags are inert.
// HyperShell therefore wraps the gRPC listener itself, so the hub can expose
// gRPC over TLS while REST stays behind an edge-terminated Route
// (specs/platform/hub-grpc-tls.spec.md).
package grpctls

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
)

// Config names the PEM files the listener serves.
type Config struct {
	CertFile string
	KeyFile  string
}

// Wrap returns a listener that terminates TLS in front of inner using the
// certificate in cfg. The certificate and key are read once now, so a missing
// or malformed file fails startup, and re-read at handshake time whenever
// either file changes on disk, so a cert-manager renewal is served on new
// connections without a restart. Only HTTP/2 is offered over ALPN, which is
// what gRPC clients negotiate.
func Wrap(inner net.Listener, cfg Config) (net.Listener, error) {
	reloader, err := newReloader(cfg)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(inner, &tls.Config{
		GetCertificate: reloader.getCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2"},
	}), nil
}

type fileStamp struct {
	modTime time.Time
	size    int64
}

type reloader struct {
	cfg  Config
	mu   sync.Mutex
	cert *tls.Certificate
	seen [2]fileStamp
}

func newReloader(cfg Config) (*reloader, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, fmt.Errorf("gRPC TLS requires both --grpc-tls-cert-file and --grpc-tls-key-file")
	}
	r := &reloader{cfg: cfg}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func stamp(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}, nil
}

// load reads both files and replaces the served certificate. The caller holds
// r.mu or is the constructor.
func (r *reloader) load() error {
	certStamp, err := stamp(r.cfg.CertFile)
	if err != nil {
		return err
	}
	keyStamp, err := stamp(r.cfg.KeyFile)
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(r.cfg.CertFile, r.cfg.KeyFile)
	if err != nil {
		return fmt.Errorf("load gRPC TLS key pair (%s, %s): %w", r.cfg.CertFile, r.cfg.KeyFile, err)
	}
	r.cert = &cert
	r.seen = [2]fileStamp{certStamp, keyStamp}
	return nil
}

func (r *reloader) changed() bool {
	certStamp, err := stamp(r.cfg.CertFile)
	if err != nil {
		return false
	}
	keyStamp, err := stamp(r.cfg.KeyFile)
	if err != nil {
		return false
	}
	return certStamp != r.seen[0] || keyStamp != r.seen[1]
}

// getCertificate serves the current key pair, reloading it first when the
// files changed. A reload that fails keeps serving the previous key pair so a
// half-written renewal cannot take the listener down.
func (r *reloader) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.changed() {
		if err := r.load(); err != nil {
			glog.Errorf("gRPC TLS certificate changed on disk but could not be reloaded; still serving the previous one: %v", err)
		} else {
			glog.Infof("gRPC TLS certificate reloaded from %s", r.cfg.CertFile)
		}
	}
	return r.cert, nil
}
