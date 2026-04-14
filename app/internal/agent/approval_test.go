package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/co2-lab/polvo/internal/agent"
)

func sampleApprovalRequest() agent.ApprovalRequest {
	return agent.ApprovalRequest{
		AgentName: "test-agent",
		SessionID: "sess-001",
		ToolName:  "write",
		ToolInput: json.RawMessage(`{"path":"foo.txt","content":"hello"}`),
		RiskLevel: "low",
		Preview:   "write to foo.txt",
	}
}

func TestAutoApproveCallback_AlwaysApproves(t *testing.T) {
	cb := agent.AutoApproveCallback{}
	dec, err := cb.RequestApproval(context.Background(), sampleApprovalRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != agent.ApprovalAllow {
		t.Errorf("expected ApprovalAllow, got %v", dec)
	}
}

func TestAutoDenyCallback_AlwaysDenies(t *testing.T) {
	cb := agent.AutoDenyCallback{}
	dec, err := cb.RequestApproval(context.Background(), sampleApprovalRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != agent.ApprovalDeny {
		t.Errorf("expected ApprovalDeny, got %v", dec)
	}
}

func TestChannelCallback_ApprovesOnChannelSend(t *testing.T) {
	requests := make(chan agent.ApprovalRequest, 1)
	responses := make(chan agent.ApprovalDecision, 1)
	cb := &agent.ChannelCallback{
		Requests:  requests,
		Responses: responses,
		Timeout:   2 * time.Second,
	}

	// Simulate user approving in background.
	go func() {
		<-requests
		responses <- agent.ApprovalAllow
	}()

	dec, err := cb.RequestApproval(context.Background(), sampleApprovalRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != agent.ApprovalAllow {
		t.Errorf("expected ApprovalAllow, got %v", dec)
	}
}

func TestChannelCallback_DeniesOnTimeout(t *testing.T) {
	requests := make(chan agent.ApprovalRequest, 1)
	responses := make(chan agent.ApprovalDecision) // nobody reads
	cb := &agent.ChannelCallback{
		Requests:  requests,
		Responses: responses,
		Timeout:   50 * time.Millisecond,
	}

	// Drain the request channel so the send succeeds; never send a response.
	go func() {
		<-requests
		// intentionally no response sent
	}()

	dec, err := cb.RequestApproval(context.Background(), sampleApprovalRequest())
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if dec != agent.ApprovalDeny {
		t.Errorf("expected ApprovalDeny on timeout, got %v", dec)
	}
}

func TestChannelCallback_DeniesOnContextCancel(t *testing.T) {
	requests := make(chan agent.ApprovalRequest, 1)
	responses := make(chan agent.ApprovalDecision)
	cb := &agent.ChannelCallback{
		Requests:  requests,
		Responses: responses,
		Timeout:   5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Drain request then cancel ctx before responding.
	go func() {
		<-requests
		cancel()
	}()

	dec, err := cb.RequestApproval(ctx, sampleApprovalRequest())
	if err == nil {
		t.Error("expected cancellation error, got nil")
	}
	if dec != agent.ApprovalDeny {
		t.Errorf("expected ApprovalDeny on cancel, got %v", dec)
	}
}
