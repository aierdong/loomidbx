package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"os"
	"unsafe"
	
	jsoniter "github.com/json-iterator/go"
	
	"loomidbx/app"
	"loomidbx/storage"
)

// json 是项目统一 JSON 编解码器配置。
var json = jsoniter.ConfigCompatibleWithStandardLibrary

// svc 是全局连接服务实例，进程启动时初始化一次。
var svc = mustInitService()

// ffiResponse 是 FFI 统一响应包装模型。
type ffiResponse struct {
	// OK 表示本次调用是否成功。
	OK bool `json:"ok"`

	// Data 为成功时返回数据载荷。
	Data interface{} `json:"data,omitempty"`

	// Error 为失败时返回的结构化错误。
	Error *app.AppError `json:"error,omitempty"`
}

// mustInitService 初始化连接服务，失败时直接终止进程。
//
// 输出：
// - *app.ConnectionService: 可供 FFI 复用的服务实例。
func mustInitService() *app.ConnectionService {
	path := os.Getenv("LOOMIDBX_STORAGE_PATH")
	store, err := storage.NewConnectionStore(path)
	if err != nil {
		panic(err)
	}
	return app.NewConnectionService(store)
}

// LDB_Version 返回当前动态库版本信息。
//
// 输出：
// - *C.char: JSON 字符串指针，需由调用方通过 LDB_FreeString 释放。
//
//export LDB_Version
func LDB_Version() *C.char {
	b, _ := json.Marshal(map[string]string{"version": "0.0.0-dev"})
	return C.CString(string(b))
}

// LDB_FreeString 释放 Go 返回给 C/Dart 的字符串内存。
//
// 输入：
// - s: 由 C.CString 分配的字符串指针。
//
//export LDB_FreeString
func LDB_FreeString(s *C.char) {
	if s == nil {
		return
	}
	C.free(unsafe.Pointer(s))
}

// LDB_SaveConnection 保存或更新连接记录（JSON FFI 入口）。
//
// 输入：
// - paramsJSON: JSON 请求字符串，映射 app.ConnectionRequest。
//
// 输出：
// - *C.char: 统一响应 JSON 字符串（ok/data/error）。
//
//export LDB_SaveConnection
func LDB_SaveConnection(paramsJSON *C.char) *C.char {
	var req app.ConnectionRequest
	if err := json.Unmarshal([]byte(C.GoString(paramsJSON)), &req); err != nil {
		return makeResponse(ffiResponse{
			OK:    false,
			Error: &app.AppError{Code: app.CodeInvalidArgument, Message: "invalid json"},
		})
	}
	id, appErr := svc.SaveConnection(context.Background(), req)
	if appErr != nil {
		return makeResponse(ffiResponse{OK: false, Error: appErr})
	}
	return makeResponse(ffiResponse{OK: true, Data: map[string]string{"id": id}})
}

// LDB_ListConnections 列出连接摘要列表（JSON FFI 入口）。
//
// 输出：
// - *C.char: 统一响应 JSON 字符串（ok/data/error）。
//
//export LDB_ListConnections
func LDB_ListConnections() *C.char {
	items, appErr := svc.ListConnections(context.Background())
	if appErr != nil {
		return makeResponse(ffiResponse{OK: false, Error: appErr})
	}
	return makeResponse(ffiResponse{OK: true, Data: items})
}

// LDB_DeleteConnection 删除连接记录（JSON FFI 入口）。
//
// 输入：
// - paramsJSON: JSON 请求字符串，映射 app.DeleteConnectionRequest。
//
// 输出：
// - *C.char: 统一响应 JSON 字符串（ok/data/error）。
//
//export LDB_DeleteConnection
func LDB_DeleteConnection(paramsJSON *C.char) *C.char {
	var req app.DeleteConnectionRequest
	if err := json.Unmarshal([]byte(C.GoString(paramsJSON)), &req); err != nil {
		return makeResponse(ffiResponse{
			OK:    false,
			Error: &app.AppError{Code: app.CodeInvalidArgument, Message: "invalid json"},
		})
	}
	appErr := svc.DeleteConnection(context.Background(), req)
	if appErr != nil {
		return makeResponse(ffiResponse{OK: false, Error: appErr})
	}
	return makeResponse(ffiResponse{OK: true, Data: map[string]string{"status": "ok"}})
}

// LDB_TestConnection 执行同步连接测试（JSON FFI 入口）。
//
// 输入：
// - paramsJSON: JSON 请求字符串，映射 app.ConnectionRequest。
//
// 输出：
// - *C.char: 统一响应 JSON 字符串（ok/data/error）。
//
//export LDB_TestConnection
func LDB_TestConnection(paramsJSON *C.char) *C.char {
	var req app.ConnectionRequest
	if err := json.Unmarshal([]byte(C.GoString(paramsJSON)), &req); err != nil {
		return makeResponse(ffiResponse{
			OK:    false,
			Error: &app.AppError{Code: app.CodeInvalidArgument, Message: "invalid json"},
		})
	}
	appErr := svc.TestConnection(context.Background(), req)
	if appErr != nil {
		return makeResponse(ffiResponse{OK: false, Error: appErr})
	}
	return makeResponse(ffiResponse{OK: true, Data: map[string]string{"status": "ok"}})
}

// makeResponse 将响应对象序列化为 C 字符串。
//
// 主要参数：
// - resp: 统一响应结构体。
func makeResponse(resp ffiResponse) *C.char {
	b, _ := json.Marshal(resp)
	return C.CString(string(b))
}

func main() {}
