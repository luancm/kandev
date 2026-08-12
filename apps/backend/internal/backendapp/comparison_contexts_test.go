package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func TestProviderLinkedChangesNormalizePersistedHosts(t *testing.T) {
	githubChange, err := githubLinkedChange(&github.TaskPR{
		ID: "gh-1", HeadHost: "https://GitHub.example/", HeadOwner: "fork", HeadRepo: "widget", HeadBranch: "feature",
		BaseHost: "https://github.example", BaseOwner: "base", BaseRepo: "widget", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("githubLinkedChange: %v", err)
	}
	if githubChange.Source.Repository.Host != "github.example" || githubChange.Base.Repository.Host != "github.example" {
		t.Fatalf("GitHub hosts = (%q, %q), want normalized host", githubChange.Source.Repository.Host, githubChange.Base.Repository.Host)
	}

	gitlabChange, err := gitlabLinkedChange(&gitlab.TaskMR{
		ID: "gl-1", SourceHost: "https://GitLab.example/", SourceProjectPath: "fork/widget", HeadBranch: "feature",
		TargetHost: "https://gitlab.example", TargetProjectPath: "group/widget", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("gitlabLinkedChange: %v", err)
	}
	if gitlabChange.Source.Repository.Host != "gitlab.example" || gitlabChange.Base.Repository.Host != "gitlab.example" {
		t.Fatalf("GitLab hosts = (%q, %q), want normalized host", gitlabChange.Source.Repository.Host, gitlabChange.Base.Repository.Host)
	}

	azureChange, err := azureLinkedChange(&azuredevops.TaskPR{
		ID: "az-1", SourceOrganizationURL: "https://dev.azure.com/fork", SourceProjectName: "Contributors", SourceRepositoryName: "widget", SourceBranch: "refs/heads/feature",
		TargetOrganizationURL: "https://dev.azure.com/base", TargetProjectName: "Product", TargetRepositoryName: "widget", TargetBranch: "refs/heads/main",
	})
	if err != nil {
		t.Fatalf("azureLinkedChange: %v", err)
	}
	if azureChange.Source.Repository.Host != "dev.azure.com" || azureChange.Base.Repository.Host != "dev.azure.com" {
		t.Fatalf("Azure hosts = (%q, %q), want normalized host", azureChange.Source.Repository.Host, azureChange.Base.Repository.Host)
	}
}

func TestProviderLinkedChangesRejectCredentialBearingHost(t *testing.T) {
	if _, err := linkedHost("https://user:secret@example.com"); err == nil {
		t.Fatal("linkedHost accepted credential-bearing provider host")
	}
	identity := gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "example.com", RepositoryPath: "acme/widget"}
	if err := identity.Validate(); err != nil {
		t.Fatalf("normalized identity should remain valid: %v", err)
	}
}
