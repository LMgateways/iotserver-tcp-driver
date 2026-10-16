/*******************************************************************************
 * Copyright 2017.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License
 * is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
 * or implied. See the License for the specific language governing permissions and limitations under
 * the License.
 *******************************************************************************/

// Package server TCP 接入服务：监听端口 → 接受设备连接 → 按行解析 JSON 报文 → 上报平台。
package server

import (
	"fmt"
	"net"
	"sync"

	"github.com/LMgateways/iotserver-sdk-go/service"

	drivercfg "github.com/LMgateways/iotserver-tcp-driver/config"
)

var GlobalDriverService *service.DriverService

// Connect 一条设备 TCP 连接。
type Connect struct {
	Conn     net.Conn
	DeviceSn string
	DeviceId string
	mu       sync.Mutex // 串行化同一连接的写（下发命令与应答可能并发）
}

// Write 向设备写数据（并发安全）。
func (c *Connect) Write(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.Conn.Write(b)
	return err
}

// TcpServer TCP 接入服务：维护 deviceSn → 连接 的映射。
type TcpServer struct {
	mu         sync.RWMutex
	ClientCons map[string]*Connect
}

var (
	tcpServer = &TcpServer{ClientCons: map[string]*Connect{}}
	listener  net.Listener
)

// Start 启动 TCP 服务（阻塞，放到 goroutine 里跑）。
//
// 与模板不同之处：
//   - 监听地址来自自定义参数（模板写死空地址，实际无法监听）；
//   - 监听失败记日志返回，不再 panic 打挂整个驱动进程。
func (t *TcpServer) Start(sd *service.DriverService, handler TcpDataHandler) {
	GlobalDriverService = sd

	addr := drivercfg.GetConfig().ListenAddr
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		sd.GetLogger().Errorf("tcp listen %s failed: %v", addr, err)
		return
	}
	listener = ln
	sd.GetLogger().Infof("tcp server listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			// 关闭监听时 Accept 返回错误，属正常退出
			sd.GetLogger().Infof("tcp accept stopped: %v", err)
			return
		}
		sd.GetLogger().Infof("tcp device connected: %s", conn.RemoteAddr())
		go serveConn(conn, handler)
	}
}

// Stop 关闭监听。
func (t *TcpServer) Stop() {
	if listener != nil {
		_ = listener.Close()
	}
}

// GetTcpServer 单例。
func GetTcpServer() *TcpServer {
	return tcpServer
}

// GetConnectByDeviceSn 按设备序列号取连接（下发命令用）。
func (t *TcpServer) GetConnectByDeviceSn(deviceSn string) *Connect {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ClientCons[deviceSn]
}

// PutConnect 登记连接：同一设备重复连接时关掉旧连接，避免连接泄漏。
func (t *TcpServer) PutConnect(deviceSn string, c *Connect) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if old, ok := t.ClientCons[deviceSn]; ok && old.Conn != c.Conn {
		_ = old.Conn.Close()
	}
	t.ClientCons[deviceSn] = c
}

// DeleteConnect 移除连接。
func (t *TcpServer) DeleteConnect(deviceSn string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.ClientCons, deviceSn)
}

// Downlink 向设备写下行数据（属性设置 / 查询 / 服务调用）。
func (t *TcpServer) Downlink(deviceSn string, payload []byte) error {
	c := t.GetConnectByDeviceSn(deviceSn)
	if c == nil {
		return fmt.Errorf("device %s is not connected", deviceSn)
	}
	return c.Write(payload)
}
