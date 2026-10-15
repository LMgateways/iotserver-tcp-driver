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

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"

	drivercfg "github.com/iotserver-org/iotserver-tcp-driver/config"
	"github.com/iotserver-org/iotserver-tcp-driver/internal/device"
)

// TcpDataHandler 业务处理回调：收到一帧合法 JSON 后由驱动实现处理（上报平台等）。
// 返回的 ack 会回写给设备；返回 nil 时默认回 {"ok":true}。
type TcpDataHandler func(c *Connect, frame map[string]interface{}) (ack map[string]interface{}, err error)

// serveConn 处理一条设备连接。
//
// 报文协议：**按行分隔的 JSON**（UTF-8，\n 结尾）。这是最简单也最好调试的自定义协议——
// 设备侧任何语言实现都只有几行，出问题时 telnet / nc 就能手工复现。
//
//	第 1 行（握手，必须）  {"sn": "device-001"}
//	后续行（数据）         {"temp": 23.5, "rpm": 1450}
//	                      {"events": {"alarm": {"code": "E01"}}}
//
// 驱动对每行回一条 {"ok":true} / {"ok":false,"error":"..."}，方便设备确认送达。
func serveConn(conn net.Conn, handler TcpDataHandler) {
	defer conn.Close()

	c := &Connect{Conn: conn}
	maxFrame := drivercfg.GetConfig().MaxFrameBytes
	if maxFrame <= 0 {
		maxFrame = 64 * 1024
	}
	reader := bufio.NewReaderSize(conn, maxFrame)

	var deviceSn string
	for {
		line, err := readLine(reader, maxFrame)
		if err != nil {
			if err != io.EOF {
				GlobalDriverService.GetLogger().Infof("tcp read %s failed: %v", conn.RemoteAddr(), err)
			}
			onConnClosed(deviceSn)
			return
		}
		if len(line) == 0 {
			continue
		}

		var frame map[string]interface{}
		if err := json.Unmarshal(line, &frame); err != nil {
			// 非法 JSON：回错误应答但不踢连接（设备可能只是发了调试文本）
			GlobalDriverService.GetLogger().Warnf("tcp invalid frame from %s: %s", conn.RemoteAddr(), string(line))
			writeJSON(c, map[string]interface{}{"ok": false, "error": "invalid json"})
			continue
		}

		// 握手帧：{"sn": "device-001"}
		if sn, ok := frame["sn"].(string); ok && sn != "" {
			if deviceSn != "" && deviceSn != sn {
				GlobalDriverService.GetLogger().Warnf("device %s re-handshake as %s on same connection", deviceSn, sn)
				GetTcpServer().DeleteConnect(deviceSn)
			}
			deviceSn = sn
			c.DeviceSn = sn
			GetTcpServer().PutConnect(sn, c)
			GlobalDriverService.GetLogger().Infof("tcp handshake ok: sn=%s from %s", sn, conn.RemoteAddr())
			writeJSON(c, map[string]interface{}{"ok": true, "sn": sn})
			continue
		}

		// 未握手就发数据：要求先发 sn
		if deviceSn == "" {
			writeJSON(c, map[string]interface{}{"ok": false, "error": `send {"sn":"..."} first`})
			continue
		}

		ack, err := handler(c, frame)
		if err != nil {
			GlobalDriverService.GetLogger().Errorf("tcp handle frame failed sn=%s: %v", deviceSn, err)
			writeJSON(c, map[string]interface{}{"ok": false, "error": err.Error()})
			continue
		}
		if ack == nil {
			ack = map[string]interface{}{"ok": true}
		}
		writeJSON(c, ack)
	}
}

// onConnClosed 连接断开收尾：登记的设备置为离线，并移除连接。
func onConnClosed(deviceSn string) {
	if deviceSn == "" {
		return
	}
	// 只有该设备当前就是这条连接时才置离线（避免重复连接时把新连接误判离线）
	if dev, ok := device.Find(deviceSn); ok {
		_ = GlobalDriverService.Offline(dev.GetDeviceId())
		dev.Online = false
	}
	GetTcpServer().DeleteConnect(deviceSn)
	GlobalDriverService.GetLogger().Infof("tcp device disconnected: sn=%s", deviceSn)
}

// readLine 读一行（\n 结尾），带长度上限。
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if len(line) > max {
		return nil, fmt.Errorf("frame exceeds max size %d bytes", max)
	}
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	if err != nil && len(line) > 0 {
		return line, nil
	}
	return line, err
}

// writeJSON 回写一帧 JSON（末尾补 \n）。
func writeJSON(c *Connect, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.Write(append(b, '\n'))
}