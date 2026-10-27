package errands

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ProfileInfo describes one agent profile a task could run an errand with.
type ProfileInfo struct {
	ID   string
	Name string
	// Agent and Model describe the profile in the parent's first-turn context.
	Agent string
	Model string
	// Passthrough marks a CLI passthrough profile. It produces no structured
	// assistant messages to capture, so errands refuse it.
	Passthrough bool
}

// ProfileCatalog lists the enabled agent profiles available to a task's
// workspace and validates one against the task's executor.
type ProfileCatalog interface {
	ListTaskProfiles(ctx context.Context, taskID string) ([]ProfileInfo, error)
	// ValidateForTaskExecutor returns an error when the profile cannot run on
	// the task's effective executor.
	ValidateForTaskExecutor(ctx context.Context, taskID, profileID string) error
}

type catalogResolver struct {
	catalog ProfileCatalog
}

// NewProfileResolver resolves errand profiles from a catalog: an exact ID
// first, then an exact name. Passthrough and executor-incompatible profiles
// are refused, and failures list up to MaxAvailableProfiles eligible names.
func NewProfileResolver(catalog ProfileCatalog) ProfileResolver {
	return &catalogResolver{catalog: catalog}
}

func (r *catalogResolver) ResolveProfile(
	ctx context.Context, taskID, selector, defaultProfileID string,
) (ResolvedProfile, error) {
	profiles, err := r.catalog.ListTaskProfiles(ctx, taskID)
	if err != nil {
		return ResolvedProfile{}, fmt.Errorf("list errand profiles: %w", err)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = defaultProfileID
	}
	matches := matchProfiles(profiles, selector)
	if len(matches) == 0 {
		return ResolvedProfile{}, r.profileError(ctx, taskID, profiles, ReasonProfileNotFound,
			"no agent profile matches the requested ID or name")
	}
	eligible := make([]ProfileInfo, 0, len(matches))
	for _, match := range matches {
		if r.eligible(ctx, taskID, match) {
			eligible = append(eligible, match)
		}
	}
	switch len(eligible) {
	case 0:
		return ResolvedProfile{}, r.profileError(ctx, taskID, profiles, ReasonProfileIncompatible,
			"the agent profile cannot run errands on this task")
	case 1:
		return ResolvedProfile{ID: eligible[0].ID, Name: eligible[0].Name}, nil
	default:
		return ResolvedProfile{}, r.profileError(ctx, taskID, profiles, ReasonProfileAmbiguous,
			"several agent profiles share the requested name; use the profile ID")
	}
}

// matchProfiles returns the profile with the exact ID, or else every profile
// with the exact name.
func matchProfiles(profiles []ProfileInfo, selector string) []ProfileInfo {
	for _, profile := range profiles {
		if profile.ID == selector {
			return []ProfileInfo{profile}
		}
	}
	var named []ProfileInfo
	for _, profile := range profiles {
		if profile.Name == selector {
			named = append(named, profile)
		}
	}
	return named
}

// CompatibleProfiles lists the profiles a parent can offer for errands: not
// passthrough, runnable on the task's executor, sorted by name, and capped at
// MaxAvailableProfiles.
func (r *catalogResolver) CompatibleProfiles(ctx context.Context, taskID string) ([]ProfileInfo, error) {
	profiles, err := r.catalog.ListTaskProfiles(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list errand profiles: %w", err)
	}
	compatible := make([]ProfileInfo, 0, len(profiles))
	for _, profile := range profiles {
		if r.eligible(ctx, taskID, profile) {
			compatible = append(compatible, profile)
		}
	}
	sort.SliceStable(compatible, func(i, j int) bool { return compatible[i].Name < compatible[j].Name })
	if len(compatible) > MaxAvailableProfiles {
		compatible = compatible[:MaxAvailableProfiles]
	}
	return compatible, nil
}

func (r *catalogResolver) eligible(ctx context.Context, taskID string, profile ProfileInfo) bool {
	if profile.Passthrough {
		return false
	}
	return r.catalog.ValidateForTaskExecutor(ctx, taskID, profile.ID) == nil
}

func (r *catalogResolver) profileError(
	ctx context.Context, taskID string, profiles []ProfileInfo, reason Reason, message string,
) *Error {
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if r.eligible(ctx, taskID, profile) {
			names = append(names, profile.Name)
		}
	}
	sort.Strings(names)
	if len(names) > MaxAvailableProfiles {
		names = names[:MaxAvailableProfiles]
	}
	return newError(reason, message, map[string]interface{}{"available": names})
}

// CompatibleProfiles lists the agent profiles a parent on the task can offer
// for errands. It is read-only and never launches anything.
func (s *Service) CompatibleProfiles(ctx context.Context, taskID string) ([]ProfileInfo, error) {
	return s.profiles.CompatibleProfiles(ctx, taskID)
}
