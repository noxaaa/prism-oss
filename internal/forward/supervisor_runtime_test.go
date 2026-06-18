package forward

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/noxaaa/prism-oss/internal/agent"
	"github.com/noxaaa/prism-oss/internal/domain"

	"nhooyr.io/websocket"
)

func TestTwoNodeRuntimesApplyConfigAndForwardTCP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetAddr, closeTarget := startTCPEchoServer(t)
	defer closeTarget()
	targetPort := mustPort(t, targetAddr)
	nodeOnePort := reserveLocalTCPPort(t)
	nodeTwoPort := reserveLocalTCPPort(t)
	acks := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept runtime websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		nodeID := "node_1"
		listenPort := nodeOnePort
		if token == "token-two" {
			nodeID = "node_2"
			listenPort = nodeTwoPort
		}
		writeRuntimeEnvelopeForForwardTest(t, request.Context(), conn, "auth_success", map[string]any{
			"agent_id": nodeID,
		})
		hello := readRuntimeEnvelopeForForwardTest(t, request.Context(), conn)
		if hello.Type != "hello" {
			t.Errorf("expected hello, got %#v", hello)
			return
		}
		writeRuntimeEnvelopeForForwardTest(t, request.Context(), conn, "config_snapshot", agent.ConfigSnapshot{
			NodeID:        nodeID,
			ConfigVersion: 1,
			Rules: []agent.RuleConfig{
				{
					ID:        "rule_" + nodeID,
					Enabled:   true,
					Protocol:  domain.ProtocolTCP,
					ListenIP:  "127.0.0.1",
					Port:      listenPort,
					MatchType: "ANY_INBOUND",
					Upstream: agent.RuleUpstreamConfig{
						Type:   "TARGET",
						Target: &agent.TargetEndpoint{ID: "target", Host: "127.0.0.1", Port: targetPort, Enabled: true},
					},
				},
			},
		})
		ack := readRuntimeEnvelopeForForwardTest(t, request.Context(), conn)
		if ack.Type != "config_ack" {
			t.Errorf("expected config ack, got %#v", ack)
			return
		}
		acks <- nodeID
		<-request.Context().Done()
	}))
	defer server.Close()

	nodeOneSupervisor := NewSupervisor()
	defer nodeOneSupervisor.Close()
	nodeTwoSupervisor := NewSupervisor()
	defer nodeTwoSupervisor.Close()
	runtimeOne := agent.NewNodeRuntime(agent.RuntimeConfig{AppName: "Runtime App", ControlPlaneURL: server.URL, AgentID: "node_1", RegistrationToken: "token-one"}, nodeOneSupervisor)
	runtimeTwo := agent.NewNodeRuntime(agent.RuntimeConfig{AppName: "Runtime App", ControlPlaneURL: server.URL, AgentID: "node_2", RegistrationToken: "token-two"}, nodeTwoSupervisor)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = runtimeOne.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		_ = runtimeTwo.Run(ctx)
	}()
	waitForRuntimeAck(t, acks)
	waitForRuntimeAck(t, acks)

	assertTCPEchoThroughPort(t, nodeOnePort, "one")
	assertTCPEchoThroughPort(t, nodeTwoPort, "two")
	cancel()
	wg.Wait()
}
