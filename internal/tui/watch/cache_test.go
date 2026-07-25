package watch

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kwatch "k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/tui/msgs"
)

func TestPodCache_AddedModifiedDeleted(t *testing.T) {
	c := newCacheFor(msgs.KindPods)

	podA := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "1"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: podA}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.PodKeyStatus] != "Pending" {
		t.Fatalf("expected Pending, got %v", rows[0][msgs.PodKeyStatus])
	}

	podAModified := podA.DeepCopy()
	podAModified.ResourceVersion = "2"
	podAModified.Status.Phase = corev1.PodRunning
	if err := c.apply(kwatch.Event{Type: kwatch.Modified, Object: podAModified}); err != nil {
		t.Fatalf("apply Modified: %v", err)
	}

	rows = c.rows("ctx1")
	if len(rows) != 1 || rows[0][msgs.PodKeyStatus] != "Running" {
		t.Fatalf("expected 1 Running row after modify, got %+v", rows)
	}

	if err := c.apply(kwatch.Event{Type: kwatch.Deleted, Object: podA}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestPodCache_StaleModifiedIgnored(t *testing.T) {
	c := newCacheFor(msgs.KindPods)

	podA := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", ResourceVersion: "5"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: podA}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	stale := podA.DeepCopy()
	stale.ResourceVersion = "3"
	stale.Status.Phase = corev1.PodFailed
	if err := c.apply(kwatch.Event{Type: kwatch.Modified, Object: stale}); err != nil {
		t.Fatalf("apply stale Modified: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 || rows[0][msgs.PodKeyStatus] != "Running" {
		t.Fatalf("expected stale redelivery to be ignored, got %+v", rows)
	}
}

func TestPodCache_ErrorEvent(t *testing.T) {
	c := newCacheFor(msgs.KindPods)
	status := &metav1.Status{Message: "boom"}
	if err := c.apply(kwatch.Event{Type: kwatch.Error, Object: status}); err == nil {
		t.Fatal("expected error from watch.Error event")
	}
}

func TestPodCache_SortedByNamespaceThenName(t *testing.T) {
	c := newCacheFor(msgs.KindPods)
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "ns2", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1", ResourceVersion: "1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns1", ResourceVersion: "1"}},
	}
	for _, p := range pods {
		if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: p}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}

	rows := c.rows("ctx1")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	wantOrder := []string{"a", "b", "z"}
	for i, want := range wantOrder {
		if rows[i][msgs.PodKeyName] != want {
			t.Fatalf("row %d: expected name %s, got %v", i, want, rows[i][msgs.PodKeyName])
		}
	}
}

func TestDeploymentCache_AddedDeleted(t *testing.T) {
	c := newCacheFor(msgs.KindDeployments)
	replicas := int32(3)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "default", ResourceVersion: "1"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: dep}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.DeployKeyReplicas] != "2/3" {
		t.Fatalf("expected replicas 2/3, got %v", rows[0][msgs.DeployKeyReplicas])
	}

	if err := c.apply(kwatch.Event{Type: kwatch.Deleted, Object: dep}); err != nil {
		t.Fatalf("apply Deleted: %v", err)
	}
	if rows := c.rows("ctx1"); len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestServiceCache_RowsIncludeEndpointPlaceholder(t *testing.T) {
	c := newCacheFor(msgs.KindServices)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default", ResourceVersion: "1"},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: svc}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.SvcKeyEndpointIPs] != EndpointIPsPlaceholder {
		t.Fatalf("expected placeholder %q, got %v", EndpointIPsPlaceholder, rows[0][msgs.SvcKeyEndpointIPs])
	}
}

func TestConfigMapCache_RowsCountKeys(t *testing.T) {
	c := newCacheFor(msgs.KindConfigMaps)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-a", Namespace: "default", ResourceVersion: "1"},
		Data:       map[string]string{"a.yaml": "1", "b.yaml": "2"},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: cm}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.ConfigMapKeyKeys] != "2" {
		t.Fatalf("expected 2 keys, got %v", rows[0][msgs.ConfigMapKeyKeys])
	}
	if rows[0][msgs.ConfigMapKeyKeyNames] != "a.yaml,b.yaml" {
		t.Fatalf("expected sorted key names, got %v", rows[0][msgs.ConfigMapKeyKeyNames])
	}
}

// TestSecretCache_NeverCarriesValues guards the security-relevant invariant
// that Secret rows carry only a key count, never Data/StringData values —
// see k8s.SecretToSecretInfo and the redactedValue policy in
// k8s.GetSecretDetail.
func TestSecretCache_NeverCarriesValues(t *testing.T) {
	c := newCacheFor(msgs.KindSecrets)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-a", Namespace: "default", ResourceVersion: "1"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("hunter2")},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: secret}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.SecretKeyKeys] != "1" {
		t.Fatalf("expected 1 key, got %v", rows[0][msgs.SecretKeyKeys])
	}
	if rows[0][msgs.SecretKeyType] != string(corev1.SecretTypeOpaque) {
		t.Fatalf("expected type Opaque, got %v", rows[0][msgs.SecretKeyType])
	}
	for _, v := range rows[0] {
		if s, ok := v.(string); ok && strings.Contains(s, "hunter2") {
			t.Fatalf("secret row leaked a raw value: %+v", rows[0])
		}
	}
}

func TestNodeCache_StatusAndRoles(t *testing.T) {
	c := newCacheFor(msgs.KindNodes)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "node-a",
			ResourceVersion: "1",
			Labels:          map[string]string{"node-role.kubernetes.io/control-plane": ""},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	if err := c.apply(kwatch.Event{Type: kwatch.Added, Object: node}); err != nil {
		t.Fatalf("apply Added: %v", err)
	}

	rows := c.rows("ctx1")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][msgs.NodeKeyStatus] != "Ready" {
		t.Fatalf("expected Ready, got %v", rows[0][msgs.NodeKeyStatus])
	}
	if rows[0][msgs.NodeKeyRoles] != "control-plane" {
		t.Fatalf("expected control-plane role, got %v", rows[0][msgs.NodeKeyRoles])
	}
}
