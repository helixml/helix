package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/mcp"

	log "github.com/sirupsen/logrus"
)

var (
	mcpServerAddr = fmt.Sprintf("127.0.0.1:%d", mcp.InternalPort)
)

func (apiServer *HelixAPIServer) startModelProxyServer(ctx context.Context) error {
	mcpServer := mcp.NewServer()

	return mcpServer.Run(ctx)
}

func testUpstreamConnection(network, addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var e error
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to connect to the upstream socket server: %s", e)
		default:
			if conn, err := net.Dial(network, addr); err != nil {
				log.Warnf("warning: test upstream %s error: %v", addr, err)
				e = err
				time.Sleep(1 * time.Second)
			} else {
				log.Infof("upstream socket server %q ok", addr)
				conn.Close()
				return nil
			}
		}
	}
}

func upstream(name, network, addr string) http.Handler {

	go func() {
		err := testUpstreamConnection(network, addr)
		if err != nil {
			log.WithFields(log.Fields{
				"error":   err,
				"addr":    addr,
				"network": network,
				"name":    name,
			}).Error("upstream socket server is not okay")
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := net.Dial(network, addr)
		if err != nil {
			log.Errorf("dial upstream error: %v", err)
			w.WriteHeader(502)
			return
		}
		if err := r.Write(peer); err != nil {
			log.Errorf("write request to upstream error: %v", err)
			w.WriteHeader(502)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(500)
			log.Errorf("w is not a hijacker: %v", err)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			w.WriteHeader(500)
			log.Errorf("hijack failed: %v", err)
			return
		}

		go func() {
			defer peer.Close()
			defer conn.Close()
			io.Copy(peer, conn)
		}()
		go func() {
			defer peer.Close()
			defer conn.Close()
			io.Copy(conn, peer)
		}()
	})
}
