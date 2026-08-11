package routing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestLoadBalancer_SelectsService(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	lb := &mockLoadBalancer{service: svc}

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})
	stage := NewLoadBalancerStage(lb)
	ctx := testContext(rule, "")

	_, result, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-4", result.Service.Model)
	require.Equal(t, "load_balancer", result.Source)
}

func TestLoadBalancer_Error(t *testing.T) {
	// The load balancer stage is terminal: a failure here has no next stage
	// to fall through to, so it is reported via err rather than a silent pass.
	lb := &mockLoadBalancer{err: errors.New("no service available")}

	rule := testRule("rule-1", "gpt-4", nil)
	stage := NewLoadBalancerStage(lb)
	ctx := testContext(rule, "")

	_, result, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.Error(t, err, "should surface the LB error")
	require.Nil(t, result)
}

func TestLoadBalancer_NilResult(t *testing.T) {
	lb := &mockLoadBalancer{service: nil} // returns nil service

	rule := testRule("rule-1", "gpt-4", nil)
	stage := NewLoadBalancerStage(lb)
	ctx := testContext(rule, "")

	_, result, err := stage.Evaluate(ctx, initialCandidateServices(ctx.Rule))
	require.Error(t, err, "a nil service with no error is still a failure to select")
	require.Nil(t, result)
}

func TestLoadBalancer_Name(t *testing.T) {
	stage := NewLoadBalancerStage(&mockLoadBalancer{})
	require.Equal(t, "load_balancer", stage.Name())
}
