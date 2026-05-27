package tools

import (
	"context"
	"fmt"
	"strings"

	gogithub "github.com/google/go-github/v60/github"
	"golang.org/x/crypto/ssh"

	"github.com/user/golovebox/internal/sandbox"
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

// CloneRepo clones a GitHub repository into the VM via SSH.
// Uses GIT_ASKPASS so the token never appears in git log or ps output.
func CloneRepo(sshClient *ssh.Client, token, owner, repo, destPath string) error {
	askpassPath := "/tmp/.golovebox_askpass.sh"
	// Single-quote the token; replace any embedded single-quotes with '\''.
	safeToken := strings.ReplaceAll(token, "'", "'\\''")
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", safeToken)

	if err := sandbox.WriteFile(sshClient, askpassPath, []byte(script)); err != nil {
		return fmt.Errorf("write askpass: %w", err)
	}
	if _, _, err := sandbox.Exec(sshClient, "chmod +x "+askpassPath); err != nil {
		return fmt.Errorf("chmod askpass: %w", err)
	}
	defer func() {
		_, _, _ = sandbox.Exec(sshClient, "rm -f "+askpassPath)
	}()

	cloneURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	cloneCmd := fmt.Sprintf("GIT_ASKPASS=%s GIT_USERNAME=x-token git clone %s %s",
		askpassPath, cloneURL, destPath)

	_, stderr, err := sandbox.Exec(sshClient, cloneCmd)
	if err != nil {
		return fmt.Errorf("git clone: %w: %s", err, stderr)
	}
	return nil
}

// OpenPR creates a Pull Request and returns its HTML URL.
func OpenPR(ctx context.Context, token, owner, repo, head, base, title, body string) (string, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)
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
