package tools

import (
	"context"
	"fmt"

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

// CloneRepo runs git clone inside the VM via SSH.
// The GitHub token is embedded in the HTTPS URL for private repo access.
func CloneRepo(sshClient *ssh.Client, token, owner, repo, destPath string) error {
	url := fmt.Sprintf("https://%s@github.com/%s/%s", token, owner, repo)
	_, _, err := sandbox.Exec(sshClient, fmt.Sprintf("git clone %s %s", url, destPath))
	return err
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
