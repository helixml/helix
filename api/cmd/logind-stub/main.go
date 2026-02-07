// logind-stub is a minimal systemd-logind D-Bus mock for Mutter.
//
// It implements just enough of the org.freedesktop.login1 interface
// for Mutter's native backend to acquire a DRM device via a lease FD.
//
// Usage: logind-stub --lease-fd=7
//
// The stub connects to the D-Bus system bus and exports:
//   - /org/freedesktop/login1 (Manager interface)
//   - /org/freedesktop/login1/session/auto (Session interface)
//   - /org/freedesktop/login1/seat/seat0 (Seat interface)
//
// When Mutter calls TakeDevice for the DRM device, the stub returns
// the lease FD. This allows Mutter to run in --display-server mode
// without real systemd-logind.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

var (
	logger  *slog.Logger
	leaseFD int = -1
)

const loginManagerIntrospect = `
<node>
  <interface name="org.freedesktop.login1.Manager">
    <method name="GetSession">
      <arg name="session_id" type="s" direction="in"/>
      <arg name="session_path" type="o" direction="out"/>
    </method>
    <method name="GetSeat">
      <arg name="seat_id" type="s" direction="in"/>
      <arg name="seat_path" type="o" direction="out"/>
    </method>
    <method name="GetSessionByPID">
      <arg name="pid" type="u" direction="in"/>
      <arg name="session_path" type="o" direction="out"/>
    </method>
  </interface>
</node>`

const loginSessionIntrospect = `
<node>
  <interface name="org.freedesktop.login1.Session">
    <method name="TakeControl">
      <arg name="force" type="b" direction="in"/>
    </method>
    <method name="ReleaseControl"/>
    <method name="TakeDevice">
      <arg name="major" type="u" direction="in"/>
      <arg name="minor" type="u" direction="in"/>
      <arg name="fd" type="h" direction="out"/>
      <arg name="inactive" type="b" direction="out"/>
    </method>
    <method name="ReleaseDevice">
      <arg name="major" type="u" direction="in"/>
      <arg name="minor" type="u" direction="in"/>
    </method>
    <method name="Activate"/>
    <signal name="PauseDevice">
      <arg name="major" type="u"/>
      <arg name="minor" type="u"/>
      <arg name="type" type="s"/>
    </signal>
    <signal name="ResumeDevice">
      <arg name="major" type="u"/>
      <arg name="minor" type="u"/>
      <arg name="fd" type="h"/>
    </signal>
    <property name="Active" type="b" access="read"/>
    <property name="Id" type="s" access="read"/>
    <property name="Seat" type="(so)" access="read"/>
    <property name="Type" type="s" access="read"/>
    <property name="VTNr" type="u" access="read"/>
  </interface>
</node>`

const loginSeatIntrospect = `
<node>
  <interface name="org.freedesktop.login1.Seat">
    <property name="Id" type="s" access="read"/>
    <property name="ActiveSession" type="(so)" access="read"/>
    <property name="CanGraphical" type="b" access="read"/>
  </interface>
</node>`

// LoginManager handles org.freedesktop.login1.Manager
type LoginManager struct{}

func (m *LoginManager) GetSession(sessionID string) (dbus.ObjectPath, *dbus.Error) {
	logger.Info("GetSession", "session_id", sessionID)
	return "/org/freedesktop/login1/session/auto", nil
}

func (m *LoginManager) GetSeat(seatID string) (dbus.ObjectPath, *dbus.Error) {
	logger.Info("GetSeat", "seat_id", seatID)
	return "/org/freedesktop/login1/seat/seat0", nil
}

func (m *LoginManager) GetSessionByPID(pid uint32) (dbus.ObjectPath, *dbus.Error) {
	logger.Info("GetSessionByPID", "pid", pid)
	return "/org/freedesktop/login1/session/auto", nil
}

// LoginSession handles org.freedesktop.login1.Session
type LoginSession struct {
	conn *dbus.Conn
}

func (s *LoginSession) TakeControl(force bool) *dbus.Error {
	logger.Info("TakeControl", "force", force)
	return nil
}

func (s *LoginSession) ReleaseControl() *dbus.Error {
	logger.Info("ReleaseControl")
	return nil
}

func (s *LoginSession) TakeDevice(major, minor uint32) (dbus.UnixFD, bool, *dbus.Error) {
	logger.Info("TakeDevice", "major", major, "minor", minor)

	// DRM device major number is 226
	if major == 226 && leaseFD >= 0 {
		// Dup the lease FD so we can return it multiple times
		newFD, err := syscall.Dup(leaseFD)
		if err != nil {
			logger.Error("Failed to dup lease FD", "err", err)
			return 0, false, dbus.MakeFailedError(err)
		}
		logger.Info("Returning lease FD for DRM device", "fd", newFD, "major", major, "minor", minor)
		return dbus.UnixFD(newFD), false, nil
	}

	// For non-DRM devices (e.g., input devices), open the real device
	devPath := fmt.Sprintf("/dev/char/%d:%d", major, minor)
	fd, err := syscall.Open(devPath, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		// Try read-only for input devices
		fd, err = syscall.Open(devPath, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
		if err != nil {
			logger.Warn("Failed to open device", "path", devPath, "err", err)
			return 0, false, dbus.MakeFailedError(err)
		}
	}
	logger.Info("Opened device", "path", devPath, "fd", fd)
	return dbus.UnixFD(fd), false, nil
}

func (s *LoginSession) ReleaseDevice(major, minor uint32) *dbus.Error {
	logger.Info("ReleaseDevice", "major", major, "minor", minor)
	return nil
}

func (s *LoginSession) Activate() *dbus.Error {
	logger.Info("Activate")
	return nil
}

// LoginSeat handles org.freedesktop.login1.Seat
type LoginSeat struct{}

func main() {
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Parse lease FD from command line
	for i, arg := range os.Args[1:] {
		if arg == "--lease-fd" && i+1 < len(os.Args)-1 {
			fd, err := strconv.Atoi(os.Args[i+2])
			if err == nil {
				leaseFD = fd
			}
		}
		if len(arg) > 11 && arg[:11] == "--lease-fd=" {
			fd, err := strconv.Atoi(arg[11:])
			if err == nil {
				leaseFD = fd
			}
		}
	}

	if leaseFD < 0 {
		fmt.Println("Usage: logind-stub --lease-fd=N")
		fmt.Println("  N is the DRM lease file descriptor")
		os.Exit(1)
	}

	logger.Info("starting logind-stub", "lease_fd", leaseFD)

	// Connect to system bus
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		logger.Error("Failed to connect to system bus", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Request the logind bus name
	reply, err := conn.RequestName("org.freedesktop.login1",
		dbus.NameFlagDoNotQueue|dbus.NameFlagReplaceExisting)
	if err != nil {
		logger.Error("Failed to request name", "err", err)
		os.Exit(1)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		logger.Error("Name already taken", "reply", reply)
		os.Exit(1)
	}
	logger.Info("Acquired org.freedesktop.login1 bus name")

	// Export Manager
	manager := &LoginManager{}
	conn.Export(manager, "/org/freedesktop/login1", "org.freedesktop.login1.Manager")
	conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{Name: "org.freedesktop.login1.Manager"},
		},
	}), "/org/freedesktop/login1", "org.freedesktop.DBus.Introspectable")

	// Export Session
	session := &LoginSession{conn: conn}
	sessionPath := dbus.ObjectPath("/org/freedesktop/login1/session/auto")
	conn.Export(session, sessionPath, "org.freedesktop.login1.Session")

	// Export session properties
	sessionProps := map[string]interface{}{
		"Active": true,
		"Id":     "auto",
		"Seat":   [2]interface{}{"seat0", dbus.ObjectPath("/org/freedesktop/login1/seat/seat0")},
		"Type":   "tty",
		"VTNr":   uint32(1),
	}
	conn.Export(
		&propHandler{props: map[string]map[string]interface{}{
			"org.freedesktop.login1.Session": sessionProps,
		}},
		sessionPath,
		"org.freedesktop.DBus.Properties",
	)

	// Export Seat
	seatPath := dbus.ObjectPath("/org/freedesktop/login1/seat/seat0")
	seat := &LoginSeat{}
	conn.Export(seat, seatPath, "org.freedesktop.login1.Seat")

	seatProps := map[string]interface{}{
		"Id":              "seat0",
		"ActiveSession":   [2]interface{}{"auto", sessionPath},
		"CanGraphical":    true,
	}
	conn.Export(
		&propHandler{props: map[string]map[string]interface{}{
			"org.freedesktop.login1.Seat": seatProps,
		}},
		seatPath,
		"org.freedesktop.DBus.Properties",
	)

	logger.Info("logind-stub ready",
		"manager", "/org/freedesktop/login1",
		"session", sessionPath,
		"seat", seatPath)

	// Wait for signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("logind-stub shutting down")
}

// propHandler implements org.freedesktop.DBus.Properties
type propHandler struct {
	props map[string]map[string]interface{}
}

func (p *propHandler) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if ifaceProps, ok := p.props[iface]; ok {
		if val, ok := ifaceProps[prop]; ok {
			return dbus.MakeVariant(val), nil
		}
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("property %s.%s not found", iface, prop))
}

func (p *propHandler) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	result := make(map[string]dbus.Variant)
	if ifaceProps, ok := p.props[iface]; ok {
		for k, v := range ifaceProps {
			result[k] = dbus.MakeVariant(v)
		}
	}
	return result, nil
}

func (p *propHandler) Set(iface, prop string, value dbus.Variant) *dbus.Error {
	return nil // ignore sets
}
