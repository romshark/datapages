package skeleton

// CI is the CI system init writes a workflow for.
type CI string

const (
	CINone CI = "none"

	// CIGitHubActions writes .github/workflows/ci.yml, rendered by [CIWorkflow].
	CIGitHubActions CI = "github-actions"
)

// CIChoices lists every CI system init accepts.
var CIChoices = []CI{CIGitHubActions, CINone}

// ParseCI returns the CI system named s.
func ParseCI(s string) (CI, error) {
	return parseChoice("CI system", CIChoices, s)
}
