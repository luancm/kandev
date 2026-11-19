package models

// ErrandExecutionIdentity namespaces an Errand's runtime transport identity.
func ErrandExecutionIdentity(errandID string) string {
	if errandID == "" {
		return ""
	}
	return "errand:" + errandID
}
