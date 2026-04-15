package schema

import "testing"

func TestComputeNextTrustState_connectionChangePriorityOverBlockingRisk(t *testing.T) {
	got := computeNextTrustState(SchemaTrustTrusted, TrustStateUpdateInput{
		ConnectionConfigChanged: true,
		HasBlockingRisk:         true,
	})
	if got != SchemaTrustPendingRescan {
		t.Fatalf("got %q want pending_rescan", got)
	}
}

func TestComputeNextTrustState_designTable(t *testing.T) {
	cases := []struct {
		name  string
		cur   SchemaTrustState
		in    TrustStateUpdateInput
		want  SchemaTrustState
	}{
		{
			name: "trusted_connection_change",
			cur:  SchemaTrustTrusted,
			in:   TrustStateUpdateInput{ConnectionConfigChanged: true},
			want: SchemaTrustPendingRescan,
		},
		{
			name: "trusted_blocking_risk",
			cur:  SchemaTrustTrusted,
			in:   TrustStateUpdateInput{HasBlockingRisk: true},
			want: SchemaTrustPendingAdjustment,
		},
		{
			name: "pending_rescan_to_trusted",
			cur:  SchemaTrustPendingRescan,
			in: TrustStateUpdateInput{
				RescanCompleted: true,
				HasBlockingRisk: false,
				SyncSucceeded:   true,
			},
			want: SchemaTrustTrusted,
		},
		{
			name: "pending_rescan_to_adjustment",
			cur:  SchemaTrustPendingRescan,
			in: TrustStateUpdateInput{
				RescanCompleted: true,
				HasBlockingRisk: true,
			},
			want: SchemaTrustPendingAdjustment,
		},
		{
			name: "pending_adjustment_to_trusted",
			cur:  SchemaTrustPendingAdjustment,
			in: TrustStateUpdateInput{
				SyncSucceeded:   true,
				HasBlockingRisk: false,
			},
			want: SchemaTrustTrusted,
		},
		{
			name: "pending_adjustment_connection_change",
			cur:  SchemaTrustPendingAdjustment,
			in: TrustStateUpdateInput{
				ConnectionConfigChanged: true,
			},
			want: SchemaTrustPendingRescan,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeNextTrustState(tc.cur, tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestBlockingReasonForTrustState(t *testing.T) {
	if blockingReasonForTrustState(SchemaTrustTrusted) != "" {
		t.Fatal("trusted should clear reason")
	}
	if blockingReasonForTrustState(SchemaTrustPendingRescan) != TrustBlockingReasonPendingRescan {
		t.Fatalf("rescan reason")
	}
	if blockingReasonForTrustState(SchemaTrustPendingAdjustment) != TrustBlockingReasonBlockingRisk {
		t.Fatalf("adjustment reason")
	}
}
