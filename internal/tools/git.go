package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v60/github"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/user/golovebox/internal/dag"
	"github.com/user/golovebox/internal/sandbox"
)

const gitTimeout = 120 * time.Second

// SyncRepo clones or pulls the repo at clonePath on the VM.
// Returns dag.ErrNeedsHuman (wrapped) on merge conflict.
func SyncRepo(ctx context.Context, client *ssh.Client, token, defaultRepo, clonePath, defaultBranch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	askpassPath, cleanup, err := writeAskpass(client, token)
	if err != nil {
		return "", fmt.Errorf("sync_repo askpass: %w", err)
	}
	defer cleanup()

	// Check if repo already cloned.
	checkCmd := fmt.Sprintf("test -d %s/.git && echo exists || echo missing", clonePath)
	existsOut, _, _ := sandbox.Exec(client, checkCmd)

	if strings.TrimSpace(existsOut) == "missing" {
		cloneCmd := fmt.Sprintf(
			"GIT_ASKPASS=%s GIT_USERNAME=x-token git clone https://github.com/%s %s 2>&1",
			askpassPath, defaultRepo, clonePath,
		)
		out, _, cloneErr := sandbox.Exec(client, cloneCmd)
		if cloneErr != nil {
			return out, fmt.Errorf("git clone: %w", cloneErr)
		}
		return out, nil
	}

	// Pull from remote.
	pullCmd := fmt.Sprintf(
		"cd %s && GIT_ASKPASS=%s GIT_USERNAME=x-token git pull origin %s 2>&1",
		clonePath, askpassPath, defaultBranch,
	)
	out, _, pullErr := sandbox.Exec(client, pullCmd)
	if pullErr != nil || isGitConflict(out) {
		return out, fmt.Errorf("merge conflict in %s: %w", defaultRepo, dag.ErrNeedsHuman)
	}
	return out, nil
}

// ExecBranch creates or checks out branchName in clonePath on the VM.
func ExecBranch(client *ssh.Client, clonePath, branchName string) (string, error) {
	cmd := fmt.Sprintf("cd %s && git checkout -b %s 2>&1", clonePath, branchName)
	out, _, err := sandbox.Exec(client, cmd)
	if err != nil {
		// Branch already exists — just switch to it.
		if strings.Contains(out, "already exists") || strings.Contains(out, "already exists") {
			switchCmd := fmt.Sprintf("cd %s && git checkout %s 2>&1", clonePath, branchName)
			out2, _, err2 := sandbox.Exec(client, switchCmd)
			return out2, err2
		}
		return out, err
	}
	return out, nil
}

// ExecPush pushes the current branch to origin with --set-upstream.
func ExecPush(client *ssh.Client, token, clonePath, branch string) (string, error) {
	askpassPath, cleanup, err := writeAskpass(client, token)
	if err != nil {
		return "", fmt.Errorf("push askpass: %w", err)
	}
	defer cleanup()

	if branch == "" {
		// Detect current branch.
		b, _, _ := sandbox.Exec(client, fmt.Sprintf("cd %s && git branch --show-current", clonePath))
		branch = strings.TrimSpace(b)
	}

	cmd := fmt.Sprintf(
		"cd %s && GIT_ASKPASS=%s GIT_USERNAME=x-token git push --set-upstream origin %s 2>&1",
		clonePath, askpassPath, branch,
	)
	out, _, pushErr := sandbox.Exec(client, cmd)
	if pushErr != nil {
		return out, fmt.Errorf("git push: %w\n%s", pushErr, dag.ErrNeedsHuman)
	}
	return out, nil
}

// ExecPR creates or updates a GitHub PR for the given branch.
// Returns the PR URL on success.
func ExecPR(ctx context.Context, token, owner, repo, head, base, titleParam, body string) (string, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)

	// Check for existing open PR on this branch.
	prs, _, err := client.PullRequests.List(ctx, owner, repo, &gogithub.PullRequestListOptions{
		State: "open",
		Head:  owner + ":" + head,
	})
	if err == nil && len(prs) > 0 {
		pr := prs[0]
		_, _, _ = client.PullRequests.Edit(ctx, owner, repo, pr.GetNumber(), &gogithub.PullRequest{
			Body: &body,
		})
		return fmt.Sprintf("PR #%d atualizado: %s", pr.GetNumber(), pr.GetHTMLURL()), nil
	}

	pr, _, err := client.PullRequests.Create(ctx, owner, repo, &gogithub.NewPullRequest{
		Title: &titleParam,
		Head:  &head,
		Base:  &base,
		Body:  &body,
	})
	if err != nil {
		return "", fmt.Errorf("create PR: %w", err)
	}
	return fmt.Sprintf("PR #%d criado: %s", pr.GetNumber(), pr.GetHTMLURL()), nil
}

// isGitConflict reports whether git output indicates a merge conflict.
func isGitConflict(output string) bool {
	for _, marker := range []string{"CONFLICT", "Automatic merge failed", "<<<<<<"} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

// writeAskpass writes a GIT_ASKPASS helper script to the VM and returns its path
// plus a cleanup function that removes the file.
func writeAskpass(client *ssh.Client, token string) (path string, cleanup func(), err error) {
	path = fmt.Sprintf("/tmp/.glb_askpass_%s.sh", uuid.New().String())
	safeToken := strings.ReplaceAll(token, "'", `'\''`)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", safeToken)
	if err = sandbox.WriteFile(client, path, []byte(script)); err != nil {
		return "", nil, err
	}
	if _, _, err = sandbox.Exec(client, "chmod +x "+path); err != nil {
		return "", nil, err
	}
	cleanup = func() { _, _, _ = sandbox.Exec(client, "rm -f "+path) }
	return path, cleanup, nil
}
