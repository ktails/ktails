package k8s

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestGetSecretDetail_RedactsValues guards the security-relevant invariant
// that a Secret's Detail Pane YAML never contains its real (even if
// base64-encoded) values — only key names, via redactedValue.
func TestGetSecretDetail_RedactsValues(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-a", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("hunter2")},
		StringData: map[string]string{"token": "super-secret-token"},
	}
	c, _ := newTestClient("ctx1", secret)

	detail, err := c.GetSecretDetail("ctx1", "default", "sec-a")
	if err != nil {
		t.Fatalf("GetSecretDetail returned error: %v", err)
	}

	if strings.Contains(detail.YAML, "hunter2") || strings.Contains(detail.YAML, "super-secret-token") {
		t.Fatalf("secret YAML leaked a raw value:\n%s", detail.YAML)
	}
	if !strings.Contains(detail.YAML, redactedValue) {
		t.Fatalf("expected redacted placeholder %q in YAML, got:\n%s", redactedValue, detail.YAML)
	}
	if !strings.Contains(detail.Summary, "password") || !strings.Contains(detail.Summary, "token") {
		t.Fatalf("expected key names (not values) in summary, got %q", detail.Summary)
	}
}
