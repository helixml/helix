package hydra

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
)

// DNSServer provides DNS resolution for Docker containers on Hydra networks.
// It resolves container names to their IPs by querying the Hydra dockerd instances.
type DNSServer struct {
	manager      *Manager
	upstreamDNS  []string
	servers      map[string]*dns.Server // Map of gateway IP -> DNS server
	serversMutex sync.RWMutex
	stopChan     chan struct{}
}

// NewDNSServer creates a new DNS server for Hydra container resolution
func NewDNSServer(manager *Manager, upstreamDNS []string) *DNSServer {
	if len(upstreamDNS) == 0 {
		upstreamDNS = []string{"8.8.8.8:53", "8.8.4.4:53"}
	}

	return &DNSServer{
		manager:     manager,
		upstreamDNS: upstreamDNS,
		servers:     make(map[string]*dns.Server),
		stopChan:    make(chan struct{}),
	}
}

// StartForInstance starts a DNS server listening on the gateway IP for a Hydra instance
func (d *DNSServer) StartForInstance(inst *DockerInstance) error {
	gateway := fmt.Sprintf("10.200.%d.1", inst.BridgeIndex)
	listenAddr := fmt.Sprintf("%s:53", gateway)

	d.serversMutex.Lock()
	defer d.serversMutex.Unlock()

	// Check if already running
	if _, exists := d.servers[gateway]; exists {
		log.Debug().Str("gateway", gateway).Msg("DNS server already running for this gateway")
		return nil
	}

	// Create DNS handler
	handler := &dnsHandler{
		manager:     d.manager,
		instance:    inst,
		upstreamDNS: d.upstreamDNS,
	}

	// Create and start DNS server
	server := &dns.Server{
		Addr:    listenAddr,
		Net:     "udp",
		Handler: handler,
	}

	go func() {
		log.Info().
			Str("listen_addr", listenAddr).
			Str("bridge_name", inst.BridgeName).
			Str("scope_id", inst.ScopeID).
			Msg("Starting Hydra DNS server for bridge")

		if err := server.ListenAndServe(); err != nil {
			// Don't log error if we're stopping
			select {
			case <-d.stopChan:
				return
			default:
				log.Error().Err(err).Str("listen_addr", listenAddr).Msg("DNS server error")
			}
		}
	}()

	d.servers[gateway] = server
	return nil
}

// StopForInstance stops the DNS server for a Hydra instance
func (d *DNSServer) StopForInstance(inst *DockerInstance) {
	gateway := fmt.Sprintf("10.200.%d.1", inst.BridgeIndex)

	d.serversMutex.Lock()
	defer d.serversMutex.Unlock()

	if server, exists := d.servers[gateway]; exists {
		log.Info().Str("gateway", gateway).Msg("Stopping Hydra DNS server")
		server.Shutdown()
		delete(d.servers, gateway)
	}
}

// StopAll stops all DNS servers
func (d *DNSServer) StopAll() {
	close(d.stopChan)

	d.serversMutex.Lock()
	defer d.serversMutex.Unlock()

	for gateway, server := range d.servers {
		log.Info().Str("gateway", gateway).Msg("Stopping Hydra DNS server")
		server.Shutdown()
	}
	d.servers = make(map[string]*dns.Server)
}

// dnsHandler handles DNS queries for a specific Hydra instance
type dnsHandler struct {
	manager     *Manager
	instance    *DockerInstance
	upstreamDNS []string
}

// ServeDNS implements dns.Handler
func (h *dnsHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			ip := h.resolveContainer(q.Name)
			if ip != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60, // Short TTL since containers may restart
					},
					A: ip,
				}
				msg.Answer = append(msg.Answer, rr)
				log.Debug().
					Str("name", q.Name).
					Str("ip", ip.String()).
					Msg("DNS: Resolved container name")
			} else {
				// Not a container name, forward to upstream
				upstream := h.forwardQuery(r)
				if upstream != nil {
					w.WriteMsg(upstream)
					return
				}
			}
		default:
			// Forward other query types to upstream
			upstream := h.forwardQuery(r)
			if upstream != nil {
				w.WriteMsg(upstream)
				return
			}
		}
	}

	w.WriteMsg(msg)
}

// resolveContainer looks up a container name in the Hydra dockerd
func (h *dnsHandler) resolveContainer(name string) net.IP {
	// Remove trailing dot from DNS name
	name = strings.TrimSuffix(name, ".")

	// Query the Hydra dockerd for container with this name
	// Container names can be:
	// - Just the name (e.g., "webapp")
	// - Compose project_service format (e.g., "myproject_webapp_1")
	// - Container ID prefix

	// Use docker inspect to resolve the container name to IP
	cmd := exec.Command("docker", "-H", "unix://"+h.instance.SocketPath,
		"inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)

	output, err := cmd.Output()
	if err != nil {
		// Try with common compose name patterns
		// Docker Compose creates names like: project_service_1
		log.Trace().
			Str("name", name).
			Err(err).
			Msg("DNS: Container not found by exact name")
		return nil
	}

	ipStr := strings.TrimSpace(string(output))
	if ipStr == "" {
		return nil
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Warn().
			Str("name", name).
			Str("ip_str", ipStr).
			Msg("DNS: Invalid IP from docker inspect")
		return nil
	}

	return ip.To4()
}

// forwardQuery forwards a DNS query to upstream servers
func (h *dnsHandler) forwardQuery(r *dns.Msg) *dns.Msg {
	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	for _, upstream := range h.upstreamDNS {
		resp, _, err := c.Exchange(r, upstream)
		if err != nil {
			log.Debug().
				Err(err).
				Str("upstream", upstream).
				Msg("DNS: Upstream query failed, trying next")
			continue
		}
		return resp
	}

	log.Warn().Msg("DNS: All upstream servers failed")
	return nil
}

// StartDNSForAllInstances starts DNS servers for all running Hydra instances
// Called when Hydra starts to ensure DNS is available for existing instances
func (d *DNSServer) StartDNSForAllInstances() {
	d.manager.mutex.RLock()
	defer d.manager.mutex.RUnlock()

	for _, inst := range d.manager.instances {
		if inst.Status == StatusRunning {
			if err := d.StartForInstance(inst); err != nil {
				log.Error().
					Err(err).
					Str("scope_id", inst.ScopeID).
					Msg("Failed to start DNS server for instance")
			}
		}
	}
}

// IntegrateDNSWithManager integrates DNS server lifecycle with the Hydra manager
// This should be called when a new instance is created or destroyed
func (m *Manager) SetDNSServer(dnsServer *DNSServer) {
	m.dnsServer = dnsServer
}

// dnsServer field needs to be added to Manager
// This will be nil if DNS is not enabled
func (m *Manager) startDNSForInstance(inst *DockerInstance) {
	if m.dnsServer != nil {
		if err := m.dnsServer.StartForInstance(inst); err != nil {
			log.Error().
				Err(err).
				Str("scope_id", inst.ScopeID).
				Msg("Failed to start DNS server for new instance")
		}
	}
}

func (m *Manager) stopDNSForInstance(inst *DockerInstance) {
	if m.dnsServer != nil {
		m.dnsServer.StopForInstance(inst)
	}
}

// UpdateDNSConfig updates DNS configuration for a running container
// This is called when BridgeDesktop completes to ensure DNS is properly configured
func (d *DNSServer) EnsureDNSRunning(ctx context.Context, inst *DockerInstance) error {
	return d.StartForInstance(inst)
}
