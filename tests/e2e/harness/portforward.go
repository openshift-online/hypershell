package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardPod opens a loopback-only ephemeral local port to a pod port. The
// returned close function is idempotent and must be called by the test.
func (c *Clients) PortForwardPod(ctx context.Context, namespace, pod string, remotePort int) (string, func(), error) {
	roundTripper, upgrader, err := spdy.RoundTripperFor(c.Config)
	if err != nil {
		return "", nil, fmt.Errorf("build port-forward transport: %w", err)
	}
	serverURL, err := url.Parse(c.Config.Host)
	if err != nil {
		return "", nil, fmt.Errorf("parse Kubernetes API host: %w", err)
	}
	serverURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", url.PathEscape(namespace), url.PathEscape(pod))
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, serverURL)

	stop := make(chan struct{})
	ready := make(chan struct{})
	forwarder, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{fmt.Sprintf("0:%d", remotePort)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return "", nil, fmt.Errorf("construct pod port-forward: %w", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- forwarder.ForwardPorts() }()

	var once sync.Once
	closeForward := func() { once.Do(func() { close(stop) }) }
	select {
	case <-ctx.Done():
		closeForward()
		return "", nil, ctx.Err()
	case err := <-errCh:
		closeForward()
		return "", nil, fmt.Errorf("start pod port-forward: %w", err)
	case <-ready:
	}
	ports, err := forwarder.GetPorts()
	if err != nil {
		closeForward()
		return "", nil, fmt.Errorf("resolve local port-forward port: %w", err)
	}
	if len(ports) != 1 {
		closeForward()
		return "", nil, fmt.Errorf("resolve local port-forward port: got %d ports", len(ports))
	}
	go func() {
		<-ctx.Done()
		closeForward()
	}()
	return fmt.Sprintf("127.0.0.1:%d", ports[0].Local), closeForward, nil
}
