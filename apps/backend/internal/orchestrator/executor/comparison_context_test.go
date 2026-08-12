package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/task/models"
)

func TestComparisonContextsForReposNormalizesRepositoryProviderHost(t *testing.T) {
	contexts, err := comparisonContextsForRepos([]*repoInfo{{
		RepositoryID: "repo-1",
		BaseBranch:   "main",
		Repository: &models.Repository{
			ID: "repo-1", Name: "widget", Provider: "github", ProviderHost: "https://GitHub.com/",
			ProviderOwner: "acme", ProviderName: "widget",
		},
	}})
	if err != nil {
		t.Fatalf("comparisonContextsForRepos: %v", err)
	}
	context, ok := contexts[""]
	if !ok || context.Target == nil {
		t.Fatalf("comparison contexts = %#v, want root context", contexts)
	}
	if context.Target.Repository.Provider != gitremote.ProviderGitHub || context.Target.Repository.Host != "github.com" || context.Target.Ref != "main" {
		t.Fatalf("comparison target = %#v, want normalized GitHub main", context.Target)
	}
}

func TestComparisonContextsForReposRejectsCredentialBearingProviderHost(t *testing.T) {
	_, err := comparisonContextsForRepos([]*repoInfo{{
		RepositoryID: "repo-1",
		Repository: &models.Repository{
			ID: "repo-1", Name: "widget", Provider: "github", ProviderHost: "https://user:secret@github.com",
			ProviderOwner: "acme", ProviderName: "widget",
		},
	}})
	if err == nil {
		t.Fatal("comparisonContextsForRepos accepted credential-bearing provider host")
	}
}
