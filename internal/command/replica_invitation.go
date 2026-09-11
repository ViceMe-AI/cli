package command

import (
	"context"

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
