// Package gitremote contains the provider-neutral identity vocabulary shared
// by Git resolvers, status consumers, and provider adapters.
package gitremote

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Provider identifies the provider rules used when comparing repository
// identities. An empty provider describes a generic Git host.
type Provider string

const (
	ProviderGeneric    Provider = ""
	ProviderGitHub     Provider = "github"
	ProviderGitLab     Provider = "gitlab"
	ProviderAzureRepos Provider = "azure_repos"
)

// RemoteRepositoryIdentity is a credential-free repository identity. The
// repository path is complete for the provider (including nested namespaces),
// while ProviderRepositoryID can carry a provider's stable identifier when a
// path is not sufficient.
type RemoteRepositoryIdentity struct {
	Provider             Provider `json:"provider,omitempty"`
	Host                 string   `json:"host,omitempty"`
	RepositoryPath       string   `json:"repository_path,omitempty"`
	ProviderRepositoryID string   `json:"provider_repository_id,omitempty"`
}

// RemoteRefIdentity combines a repository identity with one literal ref.
// Ref comparisons are always case-sensitive, even when a provider compares
// repository paths case-insensitively.
type RemoteRefIdentity struct {
	Repository RemoteRepositoryIdentity `json:"repository"`
	Ref        string                   `json:"ref"`
}

// EqualRepository compares repository identities using the provider's
// normalization rules. Hosts are case-insensitive DNS names. GitHub, GitLab,
// and Azure repository paths are case-insensitive; generic Git paths retain
// their literal case. A pair of non-empty provider IDs must also agree.
func (id RemoteRepositoryIdentity) EqualRepository(other RemoteRepositoryIdentity) bool {
	if id.Provider != other.Provider || !strings.EqualFold(id.Host, other.Host) {
		return false
	}
	if id.Host == "" || other.Host == "" {
		return false
	}
	if id.ProviderRepositoryID != "" && other.ProviderRepositoryID != "" && id.ProviderRepositoryID != other.ProviderRepositoryID {
		return false
	}
	if id.RepositoryPath == "" || other.RepositoryPath == "" {
		return id.ProviderRepositoryID != "" && id.ProviderRepositoryID == other.ProviderRepositoryID
	}

	if repositoryPathCaseInsensitive(id.Provider) {
		return strings.EqualFold(id.RepositoryPath, other.RepositoryPath)
	}
	return id.RepositoryPath == other.RepositoryPath
}

// Equal compares complete repository/ref identities. The branch/ref remains
// literal and case-sensitive by design.
func (id RemoteRefIdentity) Equal(other RemoteRefIdentity) bool {
	return id.Repository.EqualRepository(other.Repository) && id.Ref != "" && id.Ref == other.Ref
}

func repositoryPathCaseInsensitive(provider Provider) bool {
	switch provider {
	case ProviderGitHub, ProviderGitLab, ProviderAzureRepos:
		return true
	default:
		return false
	}
}

// ResolutionState describes whether a role resolved to one safe identity.
type ResolutionState string

const (
	ResolutionResolved   ResolutionState = "resolved"
	ResolutionUnresolved ResolutionState = "unresolved"
	ResolutionAmbiguous  ResolutionState = "ambiguous"
)

// ObservationState describes authoritative evidence for an exact remote ref.
type ObservationState string

const (
	ObservationUnknown ObservationState = "unknown"
	ObservationAbsent  ObservationState = "absent"
	ObservationPresent ObservationState = "present"
)

// RemoteRefObservation is one atomic observation of a resolved remote ref.
// Nil counts mean that the corresponding evidence is not known; they must not
// be converted into zero by callers.
type RemoteRefObservation struct {
	Identity         *RemoteRefIdentity `json:"identity,omitempty"`
	State            ObservationState   `json:"observation_state"`
	RemoteHeadCommit string             `json:"remote_head_commit,omitempty"`
	Ahead            *int               `json:"ahead,omitempty"`
	Behind           *int               `json:"behind,omitempty"`
}

// RemoteRole identifies one of the independent Git roles.
type RemoteRole string

const (
	ActionHeadRole       RemoteRole = "action_head"
	TrackingUpstreamRole RemoteRole = "tracking_upstream"
	ComparisonTargetRole RemoteRole = "comparison_target"
)

// RemoteRolesInput contains all executor-local values that participate in a
// role generation. Remote names are deliberately only generation evidence;
// repository/ref identity is the semantic value shared across executors.
type RemoteRolesInput struct {
	Branch               string            `json:"branch"`
	ActionHead           RemoteRefIdentity `json:"action_head"`
	TrackingUpstream     RemoteRefIdentity `json:"tracking_upstream"`
	ComparisonTarget     RemoteRefIdentity `json:"comparison_target"`
	ActionRemoteName     string            `json:"-"`
	TrackingRemoteName   string            `json:"-"`
	ComparisonRemoteName string            `json:"-"`
}

// NewGeneration returns an opaque, deterministic generation for a complete
// role selection. Length-prefixing avoids delimiter collisions between Git
// values controlled by users.
func NewGeneration(input RemoteRolesInput) string {
	hash := sha256.New()
	writeGenerationValue := func(value string) {
		_, _ = hash.Write([]byte(strconv.Itoa(len(value))))
		_, _ = hash.Write([]byte(":"))
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte(";"))
	}
	writeRepository := func(repository RemoteRepositoryIdentity) {
		writeGenerationValue(string(repository.Provider))
		writeGenerationValue(repository.Host)
		writeGenerationValue(repository.RepositoryPath)
		writeGenerationValue(repository.ProviderRepositoryID)
	}
	writeRef := func(identity RemoteRefIdentity) {
		writeRepository(identity.Repository)
		writeGenerationValue(identity.Ref)
	}

	writeGenerationValue(input.Branch)
	writeRef(input.ActionHead)
	writeRef(input.TrackingUpstream)
	writeRef(input.ComparisonTarget)
	writeGenerationValue(input.ActionRemoteName)
	writeGenerationValue(input.TrackingRemoteName)
	writeGenerationValue(input.ComparisonRemoteName)
	return hex.EncodeToString(hash.Sum(nil))
}
