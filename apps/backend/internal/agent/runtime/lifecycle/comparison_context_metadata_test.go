package lifecycle

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/gitremote"
)

func TestComparisonContextMetadataPreservesExplicitEmptyObservation(t *testing.T) {
	context, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitLab, Host: "git.example.com", RepositoryPath: "group/widget"},
		Ref:        "main",
	}, "", "generation")
	if err != nil {
		t.Fatal(err)
	}
	req := &LaunchRequest{ComparisonContexts: map[string]gitremote.ComparisonContext{"repo": context}}
	got := collectComparisonContexts(req)
	if !reflect.DeepEqual(got, req.ComparisonContexts) {
		t.Fatalf("collectComparisonContexts = %#v, want %#v", got, req.ComparisonContexts)
	}
	clear := &LaunchRequest{ComparisonContexts: map[string]gitremote.ComparisonContext{}}
	if got := collectComparisonContexts(clear); got == nil || len(got) != 0 {
		t.Fatalf("explicit empty comparison context map = %#v, want non-nil empty map", got)
	}
	metadata := buildLaunchMetadata(clear, "", "", "")
	if raw, ok := metadata[MetadataKeyComparisonContexts]; !ok || raw == nil {
		t.Fatalf("metadata missing explicit empty comparison contexts: %#v", metadata)
	}
}

func TestComparisonContextsFromMetadataRejectsCredentialIdentity(t *testing.T) {
	context := gitremote.ComparisonContext{
		Target: &gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "user:secret@github.com", RepositoryPath: "acme/widget"}, Ref: "main"},
		Update: gitremote.ComparisonContextReplace,
	}
	if _, err := comparisonContextsFromMetadata(map[string]interface{}{MetadataKeyComparisonContexts: map[string]gitremote.ComparisonContext{"": context}}); err == nil {
		t.Fatal("credential-bearing metadata was accepted")
	}
}
