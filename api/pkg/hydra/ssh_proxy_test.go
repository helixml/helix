package hydra

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxSSHProxyUpstream(t *testing.T) {
	upstream, err := SandboxSSHProxyUpstream("http://api:8080")
	require.NoError(t, err)
	require.Equal(t, "api:2224", upstream)

	_, err = SandboxSSHProxyUpstream("://bad")
	require.Error(t, err)
}

// The forwarder is a byte pipe to the control plane's SSH proxy: whatever the
// session container sends must arrive unchanged, and the reply must come back.
func TestSandboxSSHProxyForwardsBothWays(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer echo.Close()
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadString('\n')
				if err != nil {
					return
				}
				_, _ = c.Write([]byte("echo:" + line))
			}(conn)
		}
	}()

	server := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, server.StartSandboxSSHProxyWithRetry(ctx, "127.0.0.1:0", echo.Addr().String(), 10*time.Millisecond))
	defer server.stopSandboxSSHProxy()

	var addr string
	require.Eventually(t, func() bool {
		if server.sshProxyListener == nil {
			return false
		}
		addr = server.sshProxyListener.Addr().String()
		return true
	}, 2*time.Second, 10*time.Millisecond)

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("SSH-2.0-test\n"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	reply, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "echo:SSH-2.0-test\n", reply)
}

func TestSandboxSSHProxyRejectsBadUpstream(t *testing.T) {
	server := &Server{}
	require.Error(t, server.StartSandboxSSHProxyWithRetry(context.Background(), "127.0.0.1:0", "not-an-address", time.Millisecond))
}
