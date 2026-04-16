// Package ffi provides JSON adapters for FFI exports.
//
// 本包将应用服务层方法包装为符合 FFI 契约的 JSON 响应，格式如下：
//
// 成功响应: {"ok": true, "data": {...}, "error": null}
// 失败响应: {"ok": false, "data": null, "error": {"code": "...", "message": "..."}}
//
// 所有响应中不包含明文密码或令牌。
package ffi

import (
	"context"
	"encoding/json"

	"loomidbx/app"
)

// FFIResponse 是与权威文档一致的 FFI JSON 响应形状。
type FFIResponse struct {
	// Ok 表示操作是否成功。
	Ok bool `json:"ok"`

	// Data 为成功时的载荷，失败时为 nil。
	Data interface{} `json:"data,omitempty"`

	// Error 为失败时的结构化错误对象。
	Error *FFIError `json:"error,omitempty"`
}

// FFIError 是 FFI 错误对象，与 AppError 保持一致但独立定义以确保契约稳定性。
type FFIError struct {
	// Code 为稳定错误码，供 Flutter 做分支处理。
	Code string `json:"code"`

	// Reason 为可机器判定的稳定原因码（可选）。
	Reason string `json:"reason,omitempty"`

	// Message 为可读错误描述，不含敏感明文。
	Message string `json:"message"`

	// Details 为可选上下文信息，禁止放入密码/token 等敏感值。
	Details map[string]string `json:"details,omitempty"`
}

// FFIAdapter 将应用服务包装为 FFI JSON 适配器。
type FFIAdapter struct {
	// svc 为连接应用服务实例。
	svc *app.ConnectionService
}

// NewFFIAdapter 创建 FFI JSON 适配器实例。
//
// 输入：
// - svc: 连接应用服务实例。
//
// 输出：
// - *FFIAdapter: 初始化后的适配器。
func NewFFIAdapter(svc *app.ConnectionService) *FFIAdapter {
	return &FFIAdapter{svc: svc}
}

// TestConnectionJSON 执行连接测试并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 连接测试请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应（{"ok": true} 或 {"ok": false, "error": {...}}）。
func (a *FFIAdapter) TestConnectionJSON(reqJSON string) string {
	var req app.ConnectionRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}

	appErr := a.svc.TestConnection(context.Background(), req)
	if appErr != nil {
		return marshalResponse(ffiResponseFromAppError(appErr))
	}

	return marshalResponse(&FFIResponse{Ok: true})
}

// SaveConnectionJSON 保存连接并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 连接保存请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应（{"ok": true, "data": {"id": "..."}}）。
//
// 注意：响应不包含明文密码或令牌。
func (a *FFIAdapter) SaveConnectionJSON(reqJSON string) string {
	var req app.ConnectionRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}

	id, appErr := a.svc.SaveConnection(context.Background(), req)
	if appErr != nil {
		return marshalResponse(ffiResponseFromAppError(appErr))
	}

	return marshalResponse(&FFIResponse{
		Ok:   true,
		Data: map[string]string{"id": id},
	})
}

// ListConnectionsJSON 列出所有连接并返回 FFI JSON 响应。
//
// 输入：
// - 无。
//
// 输出：
// - string: FFI JSON 响应（{"ok": true, "data": [ConnectionSummary...]}）。
//
// 注意：响应不包含明文密码或令牌。
func (a *FFIAdapter) ListConnectionsJSON() string {
	list, appErr := a.svc.ListConnections(context.Background())
	if appErr != nil {
		return marshalResponse(ffiResponseFromAppError(appErr))
	}

	return marshalResponse(&FFIResponse{
		Ok:   true,
		Data: list,
	})
}

// DeleteConnectionJSON 删除连接并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 删除请求 JSON 字串，需携带 confirm_cascade=true。
//
// 输出：
// - string: FFI JSON 响应（{"ok": true} 或 {"ok": false, "error": {...}}）。
//
// 错误码：
// - CONFIRMATION_REQUIRED: 未携带 confirm_cascade=true。
// - NOT_FOUND: 连接不存在。
// - STORAGE_ERROR: 删除失败。
func (a *FFIAdapter) DeleteConnectionJSON(reqJSON string) string {
	var req app.DeleteConnectionRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}

	appErr := a.svc.DeleteConnection(context.Background(), req)
	if appErr != nil {
		return marshalResponse(ffiResponseFromAppError(appErr))
	}

	return marshalResponse(&FFIResponse{Ok: true})
}

// ffiResponseFromAppError 将应用层错误转换为 FFI 响应。
//
// 输入：
// - err: 应用层结构化错误对象。
//
// 输出：
// - *FFIResponse: 转换后的 FFI 错误响应。
func ffiResponseFromAppError(err *app.AppError) *FFIResponse {
	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    err.Code,
			Message: err.Message,
			Details: err.Details,
		},
	}
}

// ffiResponseFromParseError 将 JSON 解析错误转换为 FFI 响应。
//
// 输入：
// - err: JSON 解析错误对象。
//
// 输出：
// - *FFIResponse: 转换后的 FFI 错误响应。
func ffiResponseFromParseError(err error) *FFIResponse {
	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    app.CodeInvalidArgument,
			Message: "invalid request JSON",
			Details: map[string]string{"cause": err.Error()},
		},
	}
}

// marshalResponse 将 FFI 响应序列化为 JSON 字串。
//
// 输入：
// - resp: 待序列化的 FFI 响应对象。
//
// 输出：
// - string: 序列化后的 JSON 字串；若失败则返回最小错误响应。
func marshalResponse(resp *FFIResponse) string {
	b, err := json.Marshal(resp)
	if err != nil {
		// 序列化失败时返回最小错误响应
		return "{\"ok\":false,\"error\":{\"code\":\"INTERNAL\",\"message\":\"response serialization failed\"}}"
	}
	return string(b)
}