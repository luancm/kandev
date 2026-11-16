package errands

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type fakeCatalog struct {
	profiles     []ProfileInfo
	incompatible map[string]bool
	listErr      error
}

func (c *fakeCatalog) ListTaskProfiles(context.Context, string) ([]ProfileInfo, error) {
	return c.profiles, c.listErr
}

func (c *fakeCatalog) ValidateForTaskExecutor(_ context.Context, _, profileID string) error {
	if c.incompatible[profileID] {
		return errors.New("incompatible with the executor")
	}
	return nil
}

func resolve(catalog *fakeCatalog, selector, defaultID string) (ResolvedProfile, error) {
	return NewProfileResolver(catalog).ResolveProfile(context.Background(), "task-1", selector, defaultID)
}

// @covers AC-TASKS-ERRANDS-001.2
func TestProfileResolverMatchesIDThenExactNameThenTheDefault(t *testing.T) {
	catalog := &fakeCatalog{profiles: []ProfileInfo{
		{ID: "p-1", Name: "Reviewer"}, {ID: "p-2", Name: "Builder"}, {ID: "Builder", Name: "Shadow"},
	}}
	cases := []struct {
		selector, defaultID, want string
	}{
		{"p-2", "p-1", "p-2"},
		{"Reviewer", "p-2", "p-1"},
		{"", "p-2", "p-2"},
		{"  Reviewer ", "", "p-1"},
		{"Builder", "", "Builder"}, // an exact ID beats a name
	}
	for _, tc := range cases {
		got, err := resolve(catalog, tc.selector, tc.defaultID)
		if err != nil || got.ID != tc.want {
			t.Fatalf("resolve(%q, %q) = %+v, %v, want %s", tc.selector, tc.defaultID, got, err, tc.want)
		}
	}
}

// @covers AC-TASKS-ERRANDS-001.5
func TestProfileResolverRefusalsCarryReasonAndBoundedEligibleNames(t *testing.T) {
	var profiles []ProfileInfo
	for i := 0; i < 25; i++ {
		profiles = append(profiles, ProfileInfo{ID: fmt.Sprintf("p-%02d", i), Name: fmt.Sprintf("Agent %02d", i)})
	}
	profiles = append(profiles,
		ProfileInfo{ID: "pty", Name: "Terminal", Passthrough: true},
		ProfileInfo{ID: "remote", Name: "Remote only"},
		ProfileInfo{ID: "dup-a", Name: "Twin"}, ProfileInfo{ID: "dup-b", Name: "Twin"},
	)
	catalog := &fakeCatalog{profiles: profiles, incompatible: map[string]bool{"remote": true}}

	cases := []struct {
		selector string
		want     Reason
	}{
		{"nope", ReasonProfileNotFound},
		{"Twin", ReasonProfileAmbiguous},
		{"pty", ReasonProfileIncompatible},
		{"Terminal", ReasonProfileIncompatible},
		{"Remote only", ReasonProfileIncompatible},
	}
	for _, tc := range cases {
		_, err := resolve(catalog, tc.selector, "")
		assertReason(t, err, tc.want)
		var typed *Error
		errors.As(err, &typed)
		names, _ := typed.Details["available"].([]string)
		if len(names) != MaxAvailableProfiles {
			t.Fatalf("%s: available names = %d, want the %d-name cap", tc.selector, len(names), MaxAvailableProfiles)
		}
		for _, name := range names {
			if name == "Terminal" || name == "Remote only" {
				t.Fatalf("%s: available names list an ineligible profile %q", tc.selector, name)
			}
		}
	}
}

func TestProfileResolverPrefersTheEligibleProfileOfASharedName(t *testing.T) {
	catalog := &fakeCatalog{profiles: []ProfileInfo{
		{ID: "pty", Name: "Shared", Passthrough: true}, {ID: "real", Name: "Shared"},
	}}
	got, err := resolve(catalog, "Shared", "")
	if err != nil || got.ID != "real" {
		t.Fatalf("resolve = %+v, %v, want the non-passthrough profile", got, err)
	}
}

func TestProfileResolverSurfacesCatalogFailures(t *testing.T) {
	_, err := resolve(&fakeCatalog{listErr: errors.New("db down")}, "x", "")
	if err == nil {
		t.Fatal("a catalog failure was swallowed")
	}
	if _, typed := ReasonOf(err); typed {
		t.Fatal("a catalog failure was reported as a typed profile refusal")
	}
}

// @covers AC-TASKS-ERRANDS-009.5
func TestCompatibleProfilesListsRunnableProfilesSortedAndCapped(t *testing.T) {
	var profiles []ProfileInfo
	for i := 0; i < 25; i++ {
		profiles = append(profiles, ProfileInfo{
			ID: fmt.Sprintf("p-%02d", i), Name: fmt.Sprintf("Agent %02d", i), Agent: "claude", Model: "sonnet",
		})
	}
	profiles = append(profiles,
		ProfileInfo{ID: "pty", Name: "Terminal", Passthrough: true},
		ProfileInfo{ID: "bad", Name: "Broken"},
	)
	catalog := &fakeCatalog{profiles: profiles, incompatible: map[string]bool{"bad": true}}

	got, err := NewProfileResolver(catalog).CompatibleProfiles(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("CompatibleProfiles: %v", err)
	}
	if len(got) != MaxAvailableProfiles {
		t.Fatalf("got %d profiles, want %d", len(got), MaxAvailableProfiles)
	}
	if got[0].Name != "Agent 00" || got[0].Agent != "claude" || got[0].Model != "sonnet" {
		t.Fatalf("first profile = %+v, want Agent 00 with agent and model", got[0])
	}
	for _, profile := range got {
		if profile.Passthrough || profile.ID == "bad" || profile.ID == "pty" {
			t.Fatalf("incompatible profile %+v leaked into the list", profile)
		}
	}
}

func TestCompatibleProfilesReportsCatalogFailure(t *testing.T) {
	catalog := &fakeCatalog{listErr: errors.New("catalog down")}
	if _, err := NewProfileResolver(catalog).CompatibleProfiles(context.Background(), "task-1"); err == nil {
		t.Fatal("expected the catalog failure to surface")
	}
}
