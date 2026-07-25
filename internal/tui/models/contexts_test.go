package models

import (
	"testing"

	"charm.land/bubbles/v2/list"
)

func TestContextsInfo_ClusterFor(t *testing.T) {
	c := &ContextsInfo{list: list.New([]list.Item{}, contextDelegate{}, 0, 0)}
	c.list.SetItems([]list.Item{
		contextList{Name: "ctx-a", Cluster: "cluster-a"},
		contextList{Name: "ctx-b", Cluster: ""},
	})

	if got := c.ClusterFor("ctx-a"); got != "cluster-a" {
		t.Fatalf("expected cluster-a, got %q", got)
	}
	if got := c.ClusterFor("ctx-b"); got != "—" {
		t.Fatalf("expected em-dash fallback for an empty cluster name, got %q", got)
	}
	if got := c.ClusterFor("unknown"); got != "—" {
		t.Fatalf("expected em-dash fallback for an unknown context, got %q", got)
	}
}
