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

// Package driver TCP 协议驱动：设备以「按行分隔的 JSON」报文接入，驱动转成平台物模型消息上报。
package driver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/winc-link/hummingbird-sdk-go/commons"
	"github.com/winc-link/hummingbird-sdk-go/model"
	"github.com/winc-link/hummingbird-sdk-go/service"

	drivercfg "github.com/LMgateways/iotserver-tcp-driver/config"
	"github.com/LMgateways/iotserver-tcp-driver/internal/device"
	"github.com/LMgateways/iotserver-tcp-driver/internal/server"
)

// TcpProtocolDriver TCP 协议驱动。
type TcpProtocolDriver struct {
	sd *service.DriverService
}

// CloudPluginNotify 云插件启动/停止通知：本驱动不依赖云插件，仅记录日志。
func (dr TcpProtocolDriver) CloudPluginNotify(ctx context.Context, t commons.CloudPluginNotifyType, name string) error {
	dr.sd.GetLogger().Infof("cloud plugin notify: type=%s name=%s", t, name)
	return nil
}

// DeviceNotify 设备添加/修改/删除通知：维护驱动侧设备表。
func (dr TcpProtocolDriver) DeviceNotify(ctx context.Context, t commons.DeviceNotifyType, deviceId string, dev model.Device) error {
	switch t {
	case commons.DeviceAddNotify, commons.DeviceUpdateNotify:
		existing, ok := device.Find(dev.DeviceSn)
		online := false
		if ok {
			online = existing.Online
		}
		device.Put(&device.Device{
			DeviceId:  dev.Id,
			DeviceSn:  dev.DeviceSn,
			ProductId: dev.ProductId,
			Online:    online,
		})
		dr.sd.GetLogger().Infof("device %s: id=%s sn=%s", t, deviceId, dev.DeviceSn)
	case commons.DeviceDeleteNotify:
		// 平台已删设备：断开它的 TCP 连接并清理内存表
		if dev, ok := findDeviceById(deviceId); ok {
			server.GetTcpServer().Downlink(dev.DeviceSn, []byte(`{"cmd":"disconnect"}`))
			device.Remove(dev.DeviceSn)
		}
		dr.sd.GetLogger().Infof("device delete: id=%s", deviceId)
	default:
		dr.sd.GetLogger().Warnf("unknown device notify type: %s", t)
	}
	return nil
}

// ProductNotify 产品添加/修改/删除通知：物模型由 SDK 缓存，这里仅记录日志。
func (dr TcpProtocolDriver) ProductNotify(ctx context.Context, t commons.ProductNotifyType, productId string, product model.Product) error {
	dr.sd.GetLogger().Infof("product notify: type=%s id=%s", t, productId)
	return nil
}

// Stop 驱动退出：全部设备置离线，关闭监听。
func (dr TcpProtocolDriver) Stop(ctx context.Context) error {
	for _, dev := range device.All() {
		_ = dr.sd.Offline(dev.GetDeviceId())
	}
	server.GetTcpServer().Stop()
	dr.sd.GetLogger().Info("tcp driver stopped")
	return nil
}

// HandlePropertySet 属性设置：把平台下发的设置帧写到设备连接。
func (dr TcpProtocolDriver) HandlePropertySet(ctx context.Context, deviceId string, data model.PropertySet) error {
	return dr.downlink(deviceId, "set", data)
}

// HandlePropertyGet 属性查询：把查询帧写到设备连接，设备自行上报结果。
func (dr TcpProtocolDriver) HandlePropertyGet(ctx context.Context, deviceId string, data model.PropertyGet) error {
	return dr.downlink(deviceId, "get", data)
}

// HandleServiceExecute 服务调用：把调用帧写到设备连接。
func (dr TcpProtocolDriver) HandleServiceExecute(ctx context.Context, deviceId string, data model.ServiceExecuteRequest) error {
	return dr.downlink(deviceId, "service", data)
}

// downlink 把下行帧写到设备 TCP 连接（一行 JSON）。
func (dr TcpProtocolDriver) downlink(deviceId, action string, data interface{}) error {
	dev, ok := findDeviceById(deviceId)
	if !ok {
		return fmt.Errorf("device [%s] not found", deviceId)
	}
	payload, err := json.Marshal(map[string]interface{}{
		"cmd":  action,
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("marshal downlink: %w", err)
	}
	if err := server.GetTcpServer().Downlink(dev.DeviceSn, append(payload, '\n')); err != nil {
		return err
	}
	dr.sd.GetLogger().Infof("downlink %s sent to device %s (sn=%s)", action, deviceId, dev.DeviceSn)
	return nil
}

// handleFrame 处理一帧设备数据。
//
// 帧格式（除握手外的任意 JSON 对象）：
//
//	{"temp": 23.5, "rpm": 1450}            → 属性上报
//	{"events": {"alarm": {"code":"E01"}}}  → 事件上报
//	{"ts": 1735689600000, "temp": 23.5}    → 带采集时间戳的属性上报
func (dr *TcpProtocolDriver) handleFrame(c *server.Connect, frame map[string]interface{}) (map[string]interface{}, error) {
	dev, ok := device.Find(c.DeviceSn)
	if !ok {
		// 平台上还没这个设备：按配置决定是否自动创建
		if !drivercfg.GetConfig().AutoCreateDevice {
			return nil, fmt.Errorf("device sn=%s is not registered on platform (set AutoCreateDevice=true to auto create)", c.DeviceSn)
		}
		created, err := dr.sd.CreateDevice(model.NewAddDevice(c.DeviceSn, "", c.DeviceSn, "auto created by tcp driver", nil))
		if err != nil {
			return nil, fmt.Errorf("auto create device sn=%s: %w", c.DeviceSn, err)
		}
		dev = &device.Device{DeviceId: created.Id, DeviceSn: c.DeviceSn, ProductId: created.ProductId}
		device.Put(dev)
		c.DeviceId = created.Id
		dr.sd.GetLogger().Infof("device auto created: sn=%s id=%s", c.DeviceSn, created.Id)
	}

	// 首次收到数据视为上线
	if !dev.Online {
		if err := dr.sd.Online(dev.DeviceId); err != nil {
			dr.sd.GetLogger().Errorf("device online failed id=%s: %v", dev.DeviceId, err)
		}
		dev.Online = true
	}

	// 事件帧
	if rawEvents, ok := frame["events"].(map[string]interface{}); ok {
		for code, raw := range rawEvents {
			params, _ := raw.(map[string]interface{})
			if _, err := dr.sd.EventReport(dev.DeviceId, model.NewEventReport(false, model.NewEventData(code, params))); err != nil {
				return nil, fmt.Errorf("event report: %w", err)
			}
		}
		return map[string]interface{}{"ok": true, "reported": "events"}, nil
	}

	// 属性帧：除保留字段外都是属性
	props := make(map[string]model.PropertyData, len(frame))
	for code, value := range frame {
		if code == "sn" || code == "ts" || code == "cmd" {
			continue
		}
		props[code] = model.NewPropertyData(value)
	}
	if len(props) == 0 {
		return map[string]interface{}{"ok": true, "reported": 0}, nil
	}
	if _, err := dr.sd.PropertyReport(dev.DeviceId, model.NewPropertyReport(false, props)); err != nil {
		return nil, fmt.Errorf("property report: %w", err)
	}
	dr.sd.GetLogger().Infof("property reported device=%s count=%d", dev.DeviceId, len(props))
	return map[string]interface{}{"ok": true, "reported": len(props)}, nil
}

// findDeviceById 按 platform deviceId 反查驱动侧设备。
func findDeviceById(deviceId string) (*device.Device, bool) {
	for _, d := range device.All() {
		if d.DeviceId == deviceId {
			return d, true
		}
	}
	return nil, false
}

// loadDevices 启动时把平台上已注册的设备装进内存表。
func loadDevices(sd *service.DriverService) {
	for _, dev := range sd.GetDeviceList() {
		device.Put(&device.Device{
			DeviceId:  dev.Id,
			DeviceSn:  dev.DeviceSn,
			ProductId: dev.ProductId,
			Online:    dev.Status == commons.DeviceOnline,
		})
	}
}

// NewTcpProtocolDriver 构造 TCP 协议驱动：装载设备表 → 启动 TCP 服务。
func NewTcpProtocolDriver(sd *service.DriverService) *TcpProtocolDriver {
	dr := &TcpProtocolDriver{sd: sd}
	loadDevices(sd)
	go server.GetTcpServer().Start(sd, dr.handleFrame)
	return dr
}