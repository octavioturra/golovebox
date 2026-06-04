package tools

import (
	"context"
	"fmt"
	"strings"

	gogithub "github.com/google/go-github/v60/github"
	"github.com/google/uuid"

	"github.com/user/golovebox/core"
)

func ListIssues(ctx context.Context, token, owner, repo string) ([]*gogithub.Issue, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)
	issues, _, err := client.Issues.ListByRepo(ctx, owner, repo, &gogithub.IssueListByRepoOptions{
		State: "open",
	})
	return issues, err
}

func GetIssue(ctx context.Context, token, owner, repo string, number int) (*gogithub.Issue, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)
	issue, _, err := client.Issues.Get(ctx, owner, repo, number)
	return issue, err
}

// CloneRepo clones a GitHub repository into the VM.
// Uses GIT_ASKPASS so the token never appears in git log or ps output.
func CloneRepo(ctx context.Context, sb core.Sandbox, token, owner, repo, destPath string) error {
	askpassPath := fmt.Sprintf("/tmp/.golovebox_askpass_%s.sh", uuid.New().String())
	safeToken := strings.ReplaceAll(token, "'", `'\''`)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", safeToken)

	if err := sb.PutFile(ctx, askpassPath, []byte(script)); err != nil {
		return fmt.Errorf("write askpass: %w", err)
	}
	if _, err := sb.Exec(ctx, "chmod +x "+askpassPath); err != nil {
		return fmt.Errorf("chmod askpass: %w", err)
	}
	defer func() { _, _ = sb.Exec(context.Background(), "rm -f "+askpassPath) }()

	cloneURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	cloneCmd := fmt.Sprintf("GIT_ASKPASS=%s GIT_USERNAME=x-token git clone %s %s",
		askpassPath, cloneURL, destPath)

	o, err := sb.Exec(ctx, cloneCmd)
	if err != nil {
		return fmt.Errorf("git clone: %w: %s", err, o.Stderr)
	}
	_ = writeGitCredentials(ctx, sb, token)
	return nil
}

// OpenPR creates a Pull Request and returns its HTML URL.
func OpenPR(ctx context.Context, token, owner, repo, head, base, title, body string) (string, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)
	base, err := resolveBase(ctx, client, owner, repo, base)
	if err != nil {
		return "", err
	}
	pr, _, err := client.PullRequests.Create(ctx, owner, repo, &gogithub.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Body:  &body,
	})
	if err != nil {
		return "", err
	}
	return pr.GetHTMLURL(), nil
}
