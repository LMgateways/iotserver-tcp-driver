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

// Package device 驱动侧设备注册表：deviceSn ↔ deviceId 的内存映射。
//
// 平台的 deviceId 与设备侧可见的序列号（SN）不是一回事：设备只认自己的 SN，
// 平台内部用 deviceId 索引。这个表在两者之间做桥接，连接期间常驻内存。
package device

import "sync"

// Device 驱动侧缓存的一个设备。
type Device struct {
	DeviceId  string
	DeviceSn  string
	ProductId string
	Online    bool
}

func (d *Device) GetDeviceId() string { return d.DeviceId }
func (d *Device) GetDeviceSn() string { return d.DeviceSn }
func (d *Device) IsOnline() bool      { return d.Online }

var (
	mu   sync.RWMutex
	devs = map[string]*Device{} // deviceSn → Device
)

// Put 登记 / 更新设备。
func Put(d *Device) {
	mu.Lock()
	defer mu.Unlock()
	devs[d.DeviceSn] = d
}

// Find 按设备序列号查设备。
func Find(deviceSn string) (*Device, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := devs[deviceSn]
	return d, ok
}

// Remove 按设备序列号移除。
func Remove(deviceSn string) {
	mu.Lock()
	defer mu.Unlock()
	delete(devs, deviceSn)
}

// All 返回全部设备（启动时批量装载用）。
func All() []*Device {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]*Device, 0, len(devs))
	for _, d := range devs {
		out = append(out, d)
	}
	return out
}