package profile

import (
	"testing"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

func TestLegacyErrandModeHasErrandSurfaceWithoutQuestions(t *testing.T) {
	ctx := Legacy(mcpmode.Errand, false, nil)
	if ctx.Surface != SurfaceErrand {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceErrand)
	}
	if ctx.HasCapability(CapabilityUserQuestion) || ctx.HasCapability(CapabilityParentQuestion) {
		t.Fatalf("errand capabilities = %#v, want no question capability", ctx.Capabilities)
	}
}

func TestNewErrandSurfaceIsNotNormalizedAway(t *testing.T) {
	ctx := New(SurfaceErrand, nil, nil)
	if ctx.Surface != SurfaceErrand {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceErrand)
	}
}

func TestErrandsCapabilityIsAdditiveOnKanbanSurface(t *testing.T) {
	ctx := New(SurfaceKanbanTask, []Capability{CapabilityErrands}, nil)
	if ctx.Surface != SurfaceKanbanTask || !ctx.HasCapability(CapabilityErrands) {
		t.Fatalf("profile = %#v, want kanban surface with errands capability", ctx)
	}
	if ctx.WithoutCapability(CapabilityErrands).HasCapability(CapabilityErrands) {
		t.Fatal("errands capability was not removed")
	}
}
