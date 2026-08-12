package gitremote

import "testing"

func TestRemoteRepositoryIdentityEqualityPreservesProviderRules(t *testing.T) {
	tests := []struct {
		name  string
		left  RemoteRepositoryIdentity
		right RemoteRepositoryIdentity
		want  bool
	}{
		{
			name:  "github repository casing is insensitive",
			left:  RemoteRepositoryIdentity{Provider: ProviderGitHub, Host: "GitHub.com", RepositoryPath: "Acme/Widget"},
			right: RemoteRepositoryIdentity{Provider: ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
			want:  true,
		},
		{
			name:  "gitlab nested namespace remains complete",
			left:  RemoteRepositoryIdentity{Provider: ProviderGitLab, Host: "gitlab.example.com", RepositoryPath: "group/Widget"},
			right: RemoteRepositoryIdentity{Provider: ProviderGitLab, Host: "GITLAB.EXAMPLE.COM", RepositoryPath: "GROUP/widget"},
			want:  true,
		},
		{
			name:  "generic repository path remains case sensitive",
			left:  RemoteRepositoryIdentity{Host: "git.example.com", RepositoryPath: "Acme/Widget"},
			right: RemoteRepositoryIdentity{Host: "git.example.com", RepositoryPath: "acme/widget"},
			want:  false,
		},
		{
			name:  "provider id mismatch rejects same path",
			left:  RemoteRepositoryIdentity{Provider: ProviderAzureRepos, Host: "dev.azure.com", RepositoryPath: "org/project/repo", ProviderRepositoryID: "one"},
			right: RemoteRepositoryIdentity{Provider: ProviderAzureRepos, Host: "dev.azure.com", RepositoryPath: "org/project/repo", ProviderRepositoryID: "two"},
			want:  false,
		},
		{
			name:  "provider id can stand in for missing path",
			left:  RemoteRepositoryIdentity{Provider: ProviderAzureRepos, Host: "dev.azure.com", ProviderRepositoryID: "one"},
			right: RemoteRepositoryIdentity{Provider: ProviderAzureRepos, Host: "dev.azure.com", ProviderRepositoryID: "one"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.left.EqualRepository(tt.right); got != tt.want {
				t.Fatalf("EqualRepository() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoteRefIdentityEqualityPreservesLiteralRefCase(t *testing.T) {
	repository := RemoteRepositoryIdentity{Provider: ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"}
	left := RemoteRefIdentity{Repository: repository, Ref: "Feature/Review"}
	right := RemoteRefIdentity{Repository: repository, Ref: "feature/review"}
	if left.Equal(right) {
		t.Fatal("Equal() matched refs that differ only by literal branch case")
	}
}

func TestRemoteRoleGenerationChangesWithRoleIdentity(t *testing.T) {
	base := RemoteRolesInput{
		Branch: "feature/local",
		ActionHead: RemoteRefIdentity{
			Repository: RemoteRepositoryIdentity{Provider: ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
			Ref:        "feature/local",
		},
	}
	left := NewGeneration(base)
	base.ActionHead.Ref = "review/local"
	right := NewGeneration(base)
	if left == right {
		t.Fatal("NewGeneration() returned the same generation for different refs")
	}
}
