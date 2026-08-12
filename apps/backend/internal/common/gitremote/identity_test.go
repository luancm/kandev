package gitremote

import "testing"

func TestNormalizeHostAcceptsPersistedOriginsWithoutCredentials(t *testing.T) {
	tests := map[string]string{
		"https://GitLab.Example.test/": "gitlab.example.test",
		"gitlab.example.test:8443":     "gitlab.example.test:8443",
		"https://[::1]:443":            "::1",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := NormalizeHost(raw)
			if err != nil {
				t.Fatalf("NormalizeHost(%q): %v", raw, err)
			}
			if got != want {
				t.Fatalf("NormalizeHost(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

func TestNormalizeHostRejectsCredentialsAndPaths(t *testing.T) {
	for _, raw := range []string{
		"https://user:secret@example.com",
		"https://example.com/api",
		"https://example.com?token=secret",
		"ssh://example.com",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := NormalizeHost(raw); err == nil {
				t.Fatalf("NormalizeHost(%q) accepted unsafe host", raw)
			}
		})
	}
}

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
