package errands

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/models"
)

// seedQueuedErrand inserts a queued errand row for the test parent. A queued
// row is live without a session, so the block tests need no runtime fixture.
func seedQueuedErrand(t *testing.T, h *harness, id, title string, createdAt time.Time) {
	t.Helper()
	if err := h.store.CreateErrand(context.Background(), &models.Errand{
		ID: id, TaskID: testTask, ParentSessionID: testParent, Title: title,
		Objective: "Do " + title, Lifecycle: models.ErrandLifecycleQueued,
		ResultState: models.ErrandResultNone, CreatedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateErrand(%s): %v", id, err)
	}
}

// @covers AC-TASKS-ERRANDS-006.6
func TestStatusBlockListsAtMostTenErrandsAndCountsTheRest(t *testing.T) {
	h := newHarness(t)
	created := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 12; i++ {
		seedQueuedErrand(t, h, fmt.Sprintf("queued-%02d", i), fmt.Sprintf("audit %02d", i), created.Add(time.Duration(i)*time.Second))
	}

	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil {
		t.Fatalf("ComposeParentTurn: %v", err)
	}
	listed := 0
	for i := 0; i < 12; i++ {
		if strings.Contains(composition.Block, fmt.Sprintf("queued-%02d", i)) {
			listed++
		}
	}
	if listed != statusLineLimit {
		t.Fatalf("block lists %d errands, want %d", listed, statusLineLimit)
	}
	if !strings.Contains(composition.Block, "2"+statusOmittedLine) {
		t.Fatalf("block does not count the 2 omitted errands: %q", composition.Block)
	}
	if len(composition.Block) > statusByteLimit {
		t.Fatalf("block is %d bytes, want at most %d", len(composition.Block), statusByteLimit)
	}
}

// @covers AC-TASKS-ERRANDS-006.6
func TestStatusBlockCutsLongTitlesAtACharacterBoundary(t *testing.T) {
	h := newHarness(t)
	seedQueuedErrand(t, h, "queued-long", strings.Repeat("é", 100), time.Now().UTC())

	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil {
		t.Fatalf("ComposeParentTurn: %v", err)
	}
	if !strings.Contains(composition.Block, strings.Repeat("é", titleByteLimit/2)) {
		t.Fatalf("block does not keep the first %d bytes of the title", titleByteLimit)
	}
	if strings.Contains(composition.Block, strings.Repeat("é", titleByteLimit/2+1)) {
		t.Fatalf("block keeps more than %d bytes of the title", titleByteLimit)
	}
	if !utf8.ValidString(composition.Block) {
		t.Fatal("block cut a multi-byte character")
	}
}

// @covers AC-TASKS-ERRANDS-006.6
func TestStatusBlockIsAbsentWhileTheFlagIsOffOrNoErrandIsLive(t *testing.T) {
	h := newHarness(t)
	seedQueuedErrand(t, h, "queued-1", "audit", time.Now().UTC())
	h.service.SetEnabled(func() bool { return false })

	off, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil {
		t.Fatalf("ComposeParentTurn with the flag off: %v", err)
	}
	if off.Block != "" || off.Token != "" {
		t.Fatalf("flag off composition = %+v, want no block", off)
	}

	empty := newHarness(t)
	idle, err := empty.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil {
		t.Fatalf("ComposeParentTurn without errands: %v", err)
	}
	if idle.Block != "" || idle.Token != "" {
		t.Fatalf("parent without errands composition = %+v, want no block", idle)
	}
}
