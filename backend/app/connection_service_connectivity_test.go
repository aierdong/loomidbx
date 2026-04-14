package app_test

import (
	"context"
	"testing"

	"loomidbx/backend/app"
)

// 不可达端点应返回失败而非误判成功。
func TestConnectionDefaultTimeoutAndNoFalseSuccess(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   1,
	})
	if errObj == nil {
		t.Fatal("expect failure for unreachable endpoint")
	}
	if errObj.Code != app.CodeUpstreamUnavailable {
		t.Fatalf("unexpected code: %+v", errObj)
	}
}
