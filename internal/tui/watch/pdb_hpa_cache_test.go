package watch

import (
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kwatch "k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func TestPodDisruptionBudgetCache_AddedDeleted(t *testing.T) {
	c := newCacheFor(msgs.KindPodDisruptionBudgets)

	minAvailable := intstr.FromInt(2)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "1"},
		Spec:       policyv1.PodDisruptionBudgetSpec{MinAvailable: &minAvailable},
		Status:     policyv1.PodDisruptionBudgetStatus{CurrentHealthy: 3, DesiredHealthy: 2, DisruptionsAllowed: 1},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: pdb}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.PDBKeyMinMaxAvailable] != "minAvailable=2" {
		t.Fatalf("expected minAvailable=2, got %v", rows[0][msgs.PDBKeyMinMaxAvailable])
	}
	if rows[0][msgs.PDBKeyAllowedDisruptions] != "1" {
		t.Fatalf("expected allowedDisruptions=1, got %v", rows[0][msgs.PDBKeyAllowedDisruptions])
	}

	if err := c.apply(kwatch.Event{Type: kwatch.Deleted, Object: pdb}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestHorizontalPodAutoscalerCache_AddedDeleted(t *testing.T) {
	c := newCacheFor(msgs.KindHorizontalPodAutoscalers)

	minReplicas := int32(2)
	utilization := int32(45)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "1"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web"},
			MinReplicas:    &minReplicas,
			MaxReplicas:    10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 4,
			CurrentMetrics: []autoscalingv2.MetricStatus{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricStatus{
					Name:    "cpu",
					Current: autoscalingv2.MetricValueStatus{AverageUtilization: &utilization},
				},
			}},
		},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: hpa}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.HPAKeyReference] != "Deployment/web" {
		t.Fatalf("expected reference Deployment/web, got %v", rows[0][msgs.HPAKeyReference])
	}
	if rows[0][msgs.HPAKeyMinMax] != "2-10" {
		t.Fatalf("expected minMax 2-10, got %v", rows[0][msgs.HPAKeyMinMax])
	}
	if rows[0][msgs.HPAKeyTargets] != "cpu: 45%" {
		t.Fatalf("expected targets 'cpu: 45%%', got %v", rows[0][msgs.HPAKeyTargets])
	}

	if err := c.apply(kwatch.Event{Type: kwatch.Deleted, Object: hpa}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}
