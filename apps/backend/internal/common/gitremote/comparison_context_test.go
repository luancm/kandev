package gitremote

import "testing"

func comparisonIdentity(provider Provider, host, path, ref string) RemoteRefIdentity {
	return RemoteRefIdentity{Repository: RemoteRepositoryIdentity{Provider: provider, Host: host, RepositoryPath: path}, Ref: ref}
}

func TestSelectComparisonContextRequiresUniqueExactLinkedChange(t *testing.T) {
	action := comparisonIdentity(ProviderGitHub, "github.com", "fork/widget", "feature")
	base := comparisonIdentity(ProviderGitHub, "github.com", "kdlbs/widget", "main")

	resolved := SelectComparisonContext(ComparisonContextInput{
		ActionHead:         &action,
		LinkedChanges:      []LinkedChange{{Source: &action, Base: &base}},
		AttachedRepository: &base.Repository,
		SelectedBase:       "main",
	})
	if resolved.State != ResolutionResolved || resolved.Context.Target == nil || !resolved.Context.Target.Equal(base) {
		t.Fatalf("selection = %#v, want exact linked base", resolved)
	}

	zero := SelectComparisonContext(ComparisonContextInput{
		ActionHead:         &action,
		LinkedChanges:      []LinkedChange{{Source: ptr(comparisonIdentity(ProviderGitHub, "github.com", "other/widget", "feature")), Base: &base}},
		AttachedRepository: &base.Repository,
		SelectedBase:       "main",
	})
	if zero.State != ResolutionUnresolved {
		t.Fatalf("zero exact matches state = %q, want unresolved", zero.State)
	}

	ambiguous := SelectComparisonContext(ComparisonContextInput{
		ActionHead: &action,
		LinkedChanges: []LinkedChange{
			{Source: &action, Base: &base},
			{Source: &action, Base: &base},
		},
	})
	if ambiguous.State != ResolutionAmbiguous {
		t.Fatalf("ambiguous state = %q, want ambiguous", ambiguous.State)
	}

	incomplete := SelectComparisonContext(ComparisonContextInput{
		ActionHead:         &action,
		LinkedChanges:      []LinkedChange{{Source: &action}},
		AttachedRepository: &base.Repository,
		SelectedBase:       "main",
	})
	if incomplete.State != ResolutionUnresolved {
		t.Fatalf("incomplete state = %q, want unresolved", incomplete.State)
	}
}

func TestSelectComparisonContextFallbackPrecedence(t *testing.T) {
	attached := RemoteRepositoryIdentity{Provider: ProviderGitLab, Host: "git.example.com", RepositoryPath: "group/widget"}
	contribution := comparisonIdentity(ProviderGitLab, "git.example.com", "group/widget", "release")
	selected := SelectComparisonContext(ComparisonContextInput{
		RemoteContributionTarget: &contribution,
		AttachedRepository:       &attached,
		SelectedBase:             "main",
	})
	if selected.State != ResolutionResolved || selected.Context.Target == nil || !selected.Context.Target.Equal(contribution) {
		t.Fatalf("contribution selection = %#v", selected)
	}

	base := SelectComparisonContext(ComparisonContextInput{AttachedRepository: &attached, SelectedBase: "main"})
	if base.State != ResolutionResolved || base.Context.Target == nil || base.Context.Target.Ref != "main" {
		t.Fatalf("base selection = %#v", base)
	}

	invalid := SelectComparisonContext(ComparisonContextInput{RemoteContributionTarget: ptr(comparisonIdentity(ProviderGitLab, "git.example.com", "group/widget", "bad ref")), AttachedRepository: &attached, SelectedBase: "main"})
	if invalid.State != ResolutionUnresolved {
		t.Fatalf("invalid contribution state = %q, want unresolved", invalid.State)
	}
}

func TestComparisonContextRejectsCredentialBearingIdentity(t *testing.T) {
	identity := comparisonIdentity(ProviderGitHub, "user:secret@github.com", "kdlbs/widget", "main")
	if err := identity.Validate(); err == nil {
		t.Fatal("credential-bearing identity was accepted")
	}

	clear := ClearComparisonContext("generation")
	if err := clear.Validate(); err != nil {
		t.Fatalf("clear context invalid: %v", err)
	}
}

func TestComparisonContextRejectsMalformedRepositoryPathAndRef(t *testing.T) {
	for _, identity := range []RemoteRefIdentity{
		comparisonIdentity(ProviderGitHub, "github.com", "/acme/widget", "main"),
		comparisonIdentity(ProviderGitHub, "github.com", "acme//widget", "main"),
		comparisonIdentity(ProviderGitHub, "github.com", "acme/widget", "feature/"),
		comparisonIdentity(ProviderGitHub, "github.com", "acme/widget", "feature//review"),
	} {
		if err := identity.Validate(); err == nil {
			t.Fatalf("malformed identity was accepted: %#v", identity)
		}
	}
}

func ptr[T any](value T) *T { return &value }
