package hydra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// SandboxSSHProxyPort is where the Helix asset/sandbox SSH proxy listens on
	// the control plane, and the port Hydra mirrors it on inside the isolated
	// session bridge. Agents reach it as helix-api.internal:2224 — the same
	// hostname they already use for the API — because nothing else on the
	// bridge (not the control plane's Docker name, not its public host) is
	// routable from a session container.
	SandboxSSHProxyPort          = 2224
	SandboxSSHProxyListenAddress = SandboxNetworkGateway + ":2224"
	SandboxSSHProxyAddress       = SandboxAPIProxyHostname + ":2224"
)

// SandboxSSHProxyUpstream derives the control plane's SSH proxy address from
// the API URL Hydra already dials: same host, fixed port.
func SandboxSSHProxyUpstream(rawAPIURL string) (string, error) {
	parsed, err := url.Parse(rawAPIURL)
	if err != nil {
		return "", fmt.Errorf("parse API URL %q: %w", rawAPIURL, err)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("API URL %q has no hostname", rawAPIURL)
	}
	return net.JoinHostPort(parsed.Hostname(), fmt.Sprint(SandboxSSHProxyPort)), nil
}

// StartSandboxSSHProxyWithRetry forwards raw TCP from the session bridge to
// the control plane's SSH proxy. It is a byte pipe, not an SSH server: the
// control plane still authenticates every connection with its certificates,
// so this adds reach, not trust. Listener failures are retried like the API
// proxy's, for the same dockerd-bridge-not-ready-yet reason.
func (s *Server) StartSandboxSSHProxyWithRetry(ctx context.Context, listenAddr, upstreamAddr string, retryInterval time.Duration) error {
	if listenAddr == "" {
		listenAddr = SandboxSSHProxyListenAddress
	}
	if retryInterval <= 0 {
		retryInterval = 2 * time.Second
	}
	if _, _, err := net.SplitHostPort(upstreamAddr); err != nil {
		return fmt.Errorf("sandbox SSH proxy upstream %q: %w", upstreamAddr, err)
	}
	if s.sshProxyListener != nil || s.sshProxyRetryCancel != nil {
		return errors.New("sandbox SSH proxy already started")
	}
	retryCtx, cancel := context.WithCancel(ctx)
	s.sshProxyRetryCancel = cancel
	s.sshProxyRetryDone.Add(1)
	go func() {
		defer s.sshProxyRetryDone.Done()
		for {
			listener, err := net.Listen("tcp", listenAddr)
			if err == nil {
				s.sshProxyListener = listener
				log.Info().Str("listen", listener.Addr().String()).Str("upstream", upstreamAddr).
					Msg("Sandbox SSH proxy started")
				s.serveSandboxSSHProxy(retryCtx, listener, upstreamAddr)
				return
			}
			log.Warn().Err(err).Dur("retry_in", retryInterval).Msg("Sandbox SSH proxy listener unavailable")
			timer := time.NewTimer(retryInterval)
			select {
			case <-retryCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return nil
}

func (s *Server) serveSandboxSSHProxy(ctx context.Context, listener net.Listener, upstreamAddr string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Warn().Err(err).Msg("Sandbox SSH proxy accept failed")
			continue
		}
		go forwardTCP(ctx, conn, upstreamAddr)
	}
}

func forwardTCP(ctx context.Context, client net.Conn, upstreamAddr string) {
	defer client.Close()
	dialer := net.Dialer{Timeout: 10 * time.Second}
	upstream, err := dialer.DialContext(ctx, "tcp", upstreamAddr)
	if err != nil {
		log.Warn().Err(err).Str("upstream", upstreamAddr).Str("client", client.RemoteAddr().String()).
			Msg("Sandbox SSH proxy could not reach the control plane")
		return
	}
	defer upstream.Close()
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}
	go pipe(upstream, client)
	go pipe(client, upstream)
	select {
	case <-done:
		// Wait for the other direction to drain, or the context to end.
		select {
		case <-done:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
}

func (s *Server) stopSandboxSSHProxy() {
	// The accept loop runs on the retry goroutine, so the listener must close
	// before waiting for it — cancelling the context alone never unblocks Accept.
	if s.sshProxyRetryCancel != nil {
		s.sshProxyRetryCancel()
	}
	if s.sshProxyListener != nil {
		s.sshProxyListener.Close()
	}
	if s.sshProxyRetryCancel != nil {
		s.sshProxyRetryDone.Wait()
	}
}
