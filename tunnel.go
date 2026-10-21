package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"time"
)

type Tunnel struct {
	LocalPort int
	cmd       *exec.Cmd
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func openTunnel(c Connection) (*Tunnel, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	forward := fmt.Sprintf("%d:%s:%s", port, c.Host, c.Port)
	cmd := exec.Command("ssh",
		"-i", c.SSH.KeyPath,
		"-p", c.SSH.Port,
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ConnectTimeout=15",
		"-o", "LogLevel=ERROR",
		"-o", "BatchMode=yes",
		"-L", forward,
		c.SSH.User+"@"+c.SSH.Host)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	tunnel := &Tunnel{LocalPort: port, cmd: cmd}
	address := "127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return tunnel, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	tunnel.Close()
	return nil, fmt.Errorf("no se pudo abrir el túnel a %s por %s", c.Host, c.SSH.Host)
}

func (t *Tunnel) Close() {
	if t != nil && t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
}
