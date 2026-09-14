package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/ViceMe-AI/cli/internal/privatefile"

	"github.com/spf13/cobra"
)

type replicaInvitationKey struct{}
type replicaInvitation struct {
	id       string
	restored bool
}

func newReplicaFlowIDCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{Use: "flow-id", Short: "Generate one invitation flow identity without sending an event", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			id, err := runtime.newReplicaRequestID()
			if err != nil {
				return err
			}
			return runtime.business(struct {
				InvitationFlowID string `json:"invitationFlowId"`
			}{id})
		},
	}
}

func withReplicaInvitation(ctx context.Context, id string) context.Context {
	if !replicaUUIDPattern.MatchString(id) {
		id = ""
	}
	return context.WithValue(ctx, replicaInvitationKey{}, &replicaInvitation{id: id})
}

func replicaInvitationFrom(ctx context.Context) *replicaInvitation {
	flow, _ := ctx.Value(replicaInvitationKey{}).(*replicaInvitation)
	return flow
}

func useReplicaInvitation(ctx context.Context, runtime *Runtime, savedID string, existing bool) {
	flow := replicaInvitationFrom(ctx)
	if flow == nil {
		return
	}
	if flow.id == "" && replicaUUIDPattern.MatchString(savedID) {
		flow.id = savedID
	}
	if existing && flow.id != "" && flow.id != savedID && !flow.restored {
		runtime.client().ReportReplicaInvitation(ctx, flow.id, "RESTORED")
		flow.restored = true
	}
	// 恢复已存在的流程后，安装结果仍归属原邀请。
	if existing && replicaUUIDPattern.MatchString(savedID) {
		flow.id = savedID
	}
}

func bindReplicaInvitation(ctx context.Context, runtime *Runtime, state *replicaPurchaseState) {
	useReplicaInvitation(ctx, runtime, state.InvitationFlowID, state.OrderNo != "" || state.InvitationFlowID != "")
	if flow := replicaInvitationFrom(ctx); flow != nil && state.OrderNo == "" && state.InvitationFlowID == "" {
		state.InvitationFlowID = flow.id
	}
}

func associateReplicaInvitation(ctx context.Context, runtime *Runtime, state replicaPurchaseState) {
	if !replicaUUIDPattern.MatchString(state.InvitationFlowID) || state.OrderNo == "" {
		return
	}
	runtime.client().AssociateReplicaInvitation(ctx, state.InvitationFlowID, state.OrderNo, state.SessionID, state.SessionToken)
}

func completeReplicaInvitation(ctx context.Context, runtime *Runtime) {
	if flow := replicaInvitationFrom(ctx); flow != nil && flow.id != "" {
		runtime.client().ReportReplicaInvitation(ctx, flow.id, "INSTALL_COMPLETED")
	}
}

// 埋点与购买凭据分开持久化。摘要绑定确切凭据，旧 CLI 重写凭据后不会复用陈旧归因。
// 附属文件丢失、损坏或不可写均不影响购买或恢复。
type replicaInvitationReceipt struct {
	FlowID      string `json:"flowId"`
	StateDigest string `json:"stateDigest"`
}

func readReplicaInvitation(filename string, state []byte) string {
	data, err := readReplicaBoundedFile(filename+".invitation.json", 512)
	if err != nil {
		return ""
	}
	var receipt replicaInvitationReceipt
	digest := sha256.Sum256(state)
	if json.Unmarshal(data, &receipt) != nil || !replicaUUIDPattern.MatchString(receipt.FlowID) || receipt.StateDigest != hex.EncodeToString(digest[:]) {
		return ""
	}
	return receipt.FlowID
}

func saveReplicaInvitation(filename string, state []byte, id string) {
	if !replicaUUIDPattern.MatchString(id) {
		return
	}
	digest := sha256.Sum256(state)
	data, err := json.Marshal(replicaInvitationReceipt{FlowID: id, StateDigest: hex.EncodeToString(digest[:])})
	if err != nil {
		return
	}
	if privatefile.Write(filename+".invitation.json", data, ".replica-invitation-*.tmp") == nil {
		_ = atomicfile.SyncDirectory(filepath.Dir(filename))
	}
}
