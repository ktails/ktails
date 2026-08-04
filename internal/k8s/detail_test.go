package k8s

import (
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
)

// TestAttachEvents_ForbiddenIsHumanized guards the regression this fix
// targets: attachEvents used to be a silent swallow (getEvents's error was
// checked and then simply discarded), so an RBAC denial on core/v1 Events
// looked identical to the resource genuinely having zero events. Now it
// must set EventsError to a short, human-readable reason instead.
func TestAttachEvents_ForbiddenIsHumanized(t *testing.T) {
	c, clientset := newTestClient("ctx1")
	clientset.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "events"}, "", errors.New("denied"))
	})

	var d ResourceDetail
	c.attachEvents(&d, "ctx1", "default", "Pod", "pod-a")

	if d.EventsError == "" {
		t.Fatal("expected EventsError to be set on a Forbidden failure")
	}
	if !strings.Contains(d.EventsError, "RBAC") {
		t.Fatalf("expected a humanized RBAC message, got %q", d.EventsError)
	}
	if d.Events != nil {
		t.Fatalf("expected no events on failure, got %+v", d.Events)
	}
}

// TestAttachEvents_SuccessLeavesEventsErrorEmpty guards the non-error path:
// EventsError must stay empty when the fetch succeeds, however many events
// that turns out to be (here, zero) — otherwise every detail view would
// show the "events unavailable" message.
func TestAttachEvents_SuccessLeavesEventsErrorEmpty(t *testing.T) {
	c, _ := newTestClient("ctx1")

	var d ResourceDetail
	c.attachEvents(&d, "ctx1", "default", "Pod", "pod-a")

	if d.EventsError != "" {
		t.Fatalf("expected no EventsError on success, got %q", d.EventsError)
	}
}

// TestFormatDuration_ClampsNegativeToZero guards clock skew between this
// machine and the apiserver: a just-created object's age can come out
// slightly negative, which must render as "0s" rather than a confusing
// "-3s".
func TestFormatDuration_ClampsNegativeToZero(t *testing.T) {
	if got := formatDuration(-3 * time.Second); got != "0s" {
		t.Fatalf("formatDuration(-3s) = %q, want %q", got, "0s")
	}
	if got := formatDuration(-1 * time.Hour); got != "0s" {
		t.Fatalf("formatDuration(-1h) = %q, want %q", got, "0s")
	}
}
