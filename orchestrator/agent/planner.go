package agent

import (
	"context"
	"fmt"

	gogithub "github.com/google/go-github/v60/github"

	"github.com/user/golovebox/core"
)

// PlanFromIssue asks the LLM to elaborate a detailed task description from a GitHub issue.
// The returned string is used as input to Loop.Run.
func PlanFromIssue(ctx context.Context, c core.Completer, issue *gogithub.Issue, owner, repo string) (string, error) {
	prompt := fmt.Sprintf(
		"GitHub issue #%d in %s/%s\n\nTitle: %s\nBody:\n%s\n\n"+
			"The repository has been cloned to /root/%s.\n"+
			"Write a step-by-step plan to implement the solution, run the tests, "+
			"commit the changes on a new branch named \"fix/issue-%d\", "+
			"and open a Pull Request against main.",
		issue.GetNumber(), owner, repo,
		issue.GetTitle(),
		issue.GetBody(),
		repo,
		issue.GetNumber(),
	)

	return c.Complete(ctx, prompt)
}
