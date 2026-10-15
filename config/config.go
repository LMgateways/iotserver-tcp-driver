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

// Package config 驱动自定义配置：平台在「驱动实例」编辑页的「自定义参数」里填 JSON，
// 驱动启动时由 SDK 透传（sd.GetCustomParam()），这里解析成结构体。
package config

import (
	"encoding/json"

	"github.com/winc-link/hummingbird-sdk-go/service"
)

var baseConfig *BaseConfig

// BaseConfig TCP 驱动自定义参数，例如：
//
//	{
//	  "ListenAddr": "0.0.0.0:9000",
//	  "AutoCreateDevice": false,
//	  "MaxFrameBytes": 65536
//	}
type BaseConfig struct {
	// ListenAddr TCP 监听地址，默认 0.0.0.0:9000
	ListenAddr string
	// AutoCreateDevice 收到未注册设备的连接时，是否自动在平台创建设备（默认 false，更安全）
	AutoCreateDevice bool
	// MaxFrameBytes 单帧最大字节数（防超大帧打爆内存），默认 64KB
	MaxFrameBytes int
}

const (
	defaultListenAddr   = "0.0.0.0:9000"
	defaultMaxFrameSize = 64 * 1024
)

func InitConfig(sd *service.DriverService) {
	baseConfig = &BaseConfig{
		ListenAddr:    defaultListenAddr,
		MaxFrameBytes: defaultMaxFrameSize,
	}
	customParam := sd.GetCustomParam()
	if customParam == "" {
		return
	}
	if err := json.Unmarshal([]byte(customParam), baseConfig); err != nil {
		// 自定义参数由用户手填，格式错误时记录日志并退回默认值，不阻断驱动启动
		sd.GetLogger().Errorf("parse custom param failed, use defaults: %v", err)
		baseConfig = &BaseConfig{ListenAddr: defaultListenAddr, MaxFrameBytes: defaultMaxFrameSize}
		return
	}
	if baseConfig.ListenAddr == "" {
		baseConfig.ListenAddr = defaultListenAddr
	}
	if baseConfig.MaxFrameBytes <= 0 {
		baseConfig.MaxFrameBytes = defaultMaxFrameSize
	}
}

// GetConfig 返回驱动自定义配置（InitConfig 之后一定非 nil）。
func GetConfig() *BaseConfig {
	if baseConfig == nil {
		return &BaseConfig{ListenAddr: defaultListenAddr, MaxFrameBytes: defaultMaxFrameSize}
	}
	return baseConfig
}