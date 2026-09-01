package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
)

// ContainerManager は LXD コンテナの操作をまとめる。
// コンテナ名は「osgsuken-<username>」で固定 (1人1コンテナ)。
type ContainerManager struct {
	server lxd.InstanceServer
	image  string
}

// ContainerStatus はコンテナの状態。
type ContainerStatus string

const (
	StatusMissing  ContainerStatus = "missing"
	StatusStopped  ContainerStatus = "stopped"
	StatusRunning  ContainerStatus = "running"
	StatusStarting ContainerStatus = "starting"
)

// NewContainerManager は LXD API へ接続する。
// socketPath: /var/snap/lxd/common/lxd/unix.socket
func NewContainerManager(socketPath, image string) (*ContainerManager, error) {
	server, err := lxd.ConnectLXDUnix(socketPath, nil)
	if err != nil {
		return nil, fmt.Errorf("connect lxd: %w", err)
	}
	return &ContainerManager{server: server, image: image}, nil
}

// ContainerName は username からコンテナ名を返す。
func (c *ContainerManager) ContainerName(username string) string {
	return "osgsuken-" + username
}

// Status はコンテナの状態を返す。
func (c *ContainerManager) Status(username string) (ContainerStatus, error) {
	name := c.ContainerName(username)
	_, _, err := c.server.GetInstance(name)
	if err != nil {
		if isNotFound(err) {
			return StatusMissing, nil
		}
		return "", fmt.Errorf("get instance: %w", err)
	}

	// 起動状態は GetInstanceState で確認
	state, _, err := c.server.GetInstanceState(name)
	if err != nil {
		return "", fmt.Errorf("get instance state: %w", err)
	}
	switch state.StatusCode {
	case api.Running:
		return StatusRunning, nil
	case api.Starting:
		return StatusStarting, nil
	default:
		return StatusStopped, nil
	}
}

// Create はコンテナを作成する (起動はしない)。
// cloud-init (user-data) で起動時設定を注入する。
// security.nesting / intercept はコンテナ内で snapd (firefox 等) を動かすために必要。
func (c *ContainerManager) Create(username, userData string) error {
	name := c.ContainerName(username)
	req := api.InstancesPost{
		Name: name,
		Source: api.InstanceSource{
			Type:  "image",
			Alias: c.image,
		},
		InstancePut: api.InstancePut{
			Config: map[string]string{
				"boot.autostart":                    "false",
				"security.nesting":                  "true",
				"security.syscalls.intercept.mknod": "true",
				"security.syscalls.intercept.mount": "true",
				"user.user-data":                    userData,
			},
		},
		Type: "container",
	}
	op, err := c.server.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	return op.Wait()
}

// Start はコンテナを起動する。
func (c *ContainerManager) Start(username string) error {
	name := c.ContainerName(username)
	req := api.InstanceStatePut{Action: "start", Timeout: -1}
	op, err := c.server.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("start instance: %w", err)
	}
	return op.Wait()
}

// Stop はコンテナを停止する。
func (c *ContainerManager) Stop(username string) error {
	name := c.ContainerName(username)
	req := api.InstanceStatePut{Action: "stop", Timeout: -1}
	op, err := c.server.UpdateInstanceState(name, req, "")
	if err != nil {
		return fmt.Errorf("stop instance: %w", err)
	}
	return op.Wait()
}

// Exec はコンテナ内でコマンドを実行する (設定配布用)。
func (c *ContainerManager) Exec(username string, args []string) (string, error) {
	name := c.ContainerName(username)
	req := api.InstanceExecPost{
		Command:     append([]string{"/bin/sh", "-c"}, args...),
		WaitForWS:   false,
		Interactive: false,
	}
	op, err := c.server.ExecInstance(name, req, nil)
	if err != nil {
		return "", fmt.Errorf("exec instance: %w", err)
	}
	if err := op.Wait(); err != nil {
		return "", err
	}
	return "", nil
}

// ContainerIP はコンテナの IPv4 アドレスを返す (プロキシ用)。
// DHCP で IP が割り当てられるまで短い間隔でリトライする。
func (c *ContainerManager) ContainerIP(username string) (string, error) {
	name := c.ContainerName(username)
	var lastErr error
	for i := 0; i < 30; i++ {
		state, _, err := c.server.GetInstanceState(name)
		if err != nil {
			lastErr = fmt.Errorf("get instance state: %w", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, n := range state.Network {
			for _, addr := range n.Addresses {
				if addr.Family == "inet" && addr.Address != "127.0.0.1" {
					return addr.Address, nil
				}
			}
		}
		lastErr = fmt.Errorf("no IPv4 address for %s", name)
		time.Sleep(2 * time.Second)
	}
	return "", lastErr
}

// WaitRunning はコンテナが起動するまで待つ (タイムアウト付き)。
func (c *ContainerManager) WaitRunning(username string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := c.Status(username)
		if err != nil {
			return err
		}
		if st == StatusRunning {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for container %s to start", c.ContainerName(username))
}

// isNotFound は LXD の not found エラーを判定する。
func isNotFound(err error) bool {
	_, match := api.StatusErrorMatch(err, http.StatusNotFound)
	return match
}

const httpStatusNotFound = 404

var _ = context.Background // unused 対策
