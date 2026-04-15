package schema

// computeNextTrustState 根据当前状态与编排信号计算下一可信度状态（纯函数，便于单测覆盖 design.md 迁移表）。
//
// 输入参数：
//   - current: 当前持久化状态；空字符串按 trusted 处理。
//   - in: 来自扫描/Diff/同步编排的布尔信号。
//
// 返回值：下一状态枚举。
func computeNextTrustState(current SchemaTrustState, in TrustStateUpdateInput) SchemaTrustState {
	if current == "" {
		current = SchemaTrustTrusted
	}
	// 连接配置变更优先进入 pending_rescan（覆盖阻断风险等其它触发）。
	if in.ConnectionConfigChanged {
		return SchemaTrustPendingRescan
	}
	switch current {
	case SchemaTrustTrusted:
		if in.HasBlockingRisk {
			return SchemaTrustPendingAdjustment
		}
		return SchemaTrustTrusted

	case SchemaTrustPendingRescan:
		if in.RescanCompleted && in.HasBlockingRisk {
			return SchemaTrustPendingAdjustment
		}
		if in.RescanCompleted && !in.HasBlockingRisk && in.SyncSucceeded {
			return SchemaTrustTrusted
		}
		return SchemaTrustPendingRescan

	case SchemaTrustPendingAdjustment:
		if in.SyncSucceeded && !in.HasBlockingRisk {
			return SchemaTrustTrusted
		}
		return SchemaTrustPendingAdjustment

	default:
		return current
	}
}

// blockingReasonForTrustState 返回写入 ldb_connections.extra 的 schema_last_blocking_reason；trusted 时为空串。
func blockingReasonForTrustState(s SchemaTrustState) string {
	switch s {
	case SchemaTrustTrusted:
		return ""
	case SchemaTrustPendingRescan:
		return TrustBlockingReasonPendingRescan
	case SchemaTrustPendingAdjustment:
		return TrustBlockingReasonBlockingRisk
	default:
		return ""
	}
}
