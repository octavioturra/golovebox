package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v60/github"
	"github.com/google/uuid"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/orchestrator/dag"
)

const gitTimeout = 120 * time.Second

// SyncRepo clones or pulls the repo at clonePath on the VM.
// Returns dag.ErrNeedsHuman (wrapped) on merge conflict.
func SyncRepo(ctx context.Context, sb core.Sandbox, token, defaultRepo, clonePath, defaultBranch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	askpassPath, cleanup, err := writeAskpass(ctx, sb, token)
	if err != nil {
		return "", fmt.Errorf("sync_repo askpass: %w", err)
	}
	defer cleanup()

	// Persist credentials so the agent's bare `git push` authenticates.
	_ = writeGitCredentials(ctx, sb, token)

	checkCmd := fmt.Sprintf("test -d %s/.git && echo exists || echo missing", clonePath)
	o, _ := sb.Exec(ctx, checkCmd)

	if strings.TrimSpace(o.Stdout) == "missing" {
		mkdirCmd := fmt.Sprintf("mkdir -p $(dirname %s)", clonePath)
		_, _ = sb.Exec(ctx, mkdirCmd)

		cloneCmd := fmt.Sprintf(
			"GIT_ASKPASS=%s GIT_USERNAME=x-token git clone https://github.com/%s %s 2>&1",
			askpassPath, defaultRepo, clonePath,
		)
		o2, cloneErr := sb.Exec(ctx, cloneCmd)
		if cloneErr != nil {
			return o2.Stdout, fmt.Errorf("git clone: %w", cloneErr)
		}
		return o2.Stdout, nil
	}

	pullCmd := fmt.Sprintf(
		"cd %s && GIT_ASKPASS=%s GIT_USERNAME=x-token git pull origin %s 2>&1",
		clonePath, askpassPath, defaultBranch,
	)
	o3, pullErr := sb.Exec(ctx, pullCmd)
	if pullErr != nil || isGitConflict(o3.Stdout) {
		return o3.Stdout, fmt.Errorf("merge conflict in %s: %w", defaultRepo, dag.ErrNeedsHuman)
	}
	return o3.Stdout, nil
}

// ExecBranch creates or checks out branchName in clonePath on the VM.
func ExecBranch(ctx context.Context, sb core.Sandbox, clonePath, branchName string) (string, error) {
	o, err := sb.Exec(ctx, fmt.Sprintf("cd %s && git checkout -b %s 2>&1", clonePath, branchName))
	if err != nil {
		if strings.Contains(o.Stdout, "already exists") {
			o2, err2 := sb.Exec(ctx, fmt.Sprintf("cd %s && git checkout %s 2>&1", clonePath, branchName))
			return o2.Stdout, err2
		}
		return o.Stdout, err
	}
	return o.Stdout, nil
}

// ExecCommit stages all changes and commits them on the current branch.
// It is a no-op (without error) when the working tree is clean, so a push can
// still proceed against previously committed work.
func ExecCommit(ctx context.Context, sb core.Sandbox, clonePath, message string) (string, error) {
	if message == "" {
		message = "chore: golovebox automated commit"
	}
	// Ensure git has an identity inside the sandbox so commit doesn't fail.
	_, _ = sb.Exec(ctx, fmt.Sprintf(
		"cd %s && git config user.email >/dev/null 2>&1 || git config user.email golovebox@local; "+
			"git config user.name >/dev/null 2>&1 || git config user.name golovebox",
		clonePath,
	))

	status, _ := sb.Exec(ctx, fmt.Sprintf("cd %s && git status --porcelain", clonePath))
	if strings.TrimSpace(status.Stdout) == "" {
		return "nothing to commit, working tree clean", nil
	}

	safeMsg := strings.ReplaceAll(message, "'", `'\''`)
	o, err := sb.Exec(ctx, fmt.Sprintf(
		"cd %s && git add -A && git commit -m '%s' 2>&1",
		clonePath, safeMsg,
	))
	if err != nil {
		return o.Stdout, fmt.Errorf("git commit: %w", err)
	}
	return o.Stdout, nil
}

// ExecPush pushes the current branch to origin with --set-upstream.
func ExecPush(ctx context.Context, sb core.Sandbox, token, clonePath, branch string) (string, error) {
	askpassPath, cleanup, err := writeAskpass(ctx, sb, token)
	if err != nil {
		return "", fmt.Errorf("push askpass: %w", err)
	}
	defer cleanup()

	if branch == "" {
		o, _ := sb.Exec(ctx, fmt.Sprintf("cd %s && git branch --show-current", clonePath))
		branch = strings.TrimSpace(o.Stdout)
	}

	o, pushErr := sb.Exec(ctx, fmt.Sprintf(
		"cd %s && GIT_ASKPASS=%s GIT_USERNAME=x-token git push --set-upstream origin %s 2>&1",
		clonePath, askpassPath, branch,
	))
	if pushErr != nil {
		return o.Stdout, fmt.Errorf("git push: %w\n%s", pushErr, dag.ErrNeedsHuman)
	}
	return o.Stdout, nil
}

// ExecCommitPush stages all changes, commits, and pushes to origin.
// A clean working tree is not an error — push still runs so the branch exists on remote.
func ExecCommitPush(ctx context.Context, sb core.Sandbox, token, clonePath, branch, commitMsg string) error {
	if _, err := ExecCommit(ctx, sb, clonePath, commitMsg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if _, err := ExecPush(ctx, sb, token, clonePath, branch); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// ExecPR creates or updates a GitHub PR for the given branch.
func ExecPR(ctx context.Context, token, owner, repo, head, base, titleParam, body string) (string, error) {
	return ExecPRWithBase(ctx, token, owner, repo, head, base, titleParam, body, "")
}

// ExecPRWithBase is the testable variant of ExecPR that accepts an optional
// base URL for the GitHub API client (used in unit tests with httptest.Server).
// Pass an empty string for production use.
func ExecPRWithBase(ctx context.Context, token, owner, repo, head, base, titleParam, body, apiBaseURL string) (string, error) {
	client := gogithub.NewClient(nil).WithAuthToken(token)
	if apiBaseURL != "" {
		var err error
		client, err = client.WithEnterpriseURLs(apiBaseURL, apiBaseURL)
		if err != nil {
			return "", fmt.Errorf("github client base url: %w", err)
		}
	}

	base, err := resolveBase(ctx, client, owner, repo, base)
	if err != nil {
		return "", err
	}

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

// resolveBase returns the base branch to target for a PR. When base is empty,
// it queries the repository's actual default branch (which may be "master",
// "main", or anything else) instead of assuming "main" — avoiding the GitHub
// 422 "base invalid" error on repos whose default branch differs.
func resolveBase(ctx context.Context, client *gogithub.Client, owner, repo, base string) (string, error) {
	if base != "" {
		return base, nil
	}
	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("resolve default branch for %s/%s: %w", owner, repo, err)
	}
	return r.GetDefaultBranch(), nil
}

func isGitConflict(output string) bool {
	for _, marker := range []string{"CONFLICT", "Automatic merge failed", "<<<<<<"} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func writeGitCredentials(ctx context.Context, sb core.Sandbox, token string) error {
	if token == "" {
		return nil
	}
	line := fmt.Sprintf("https://x-token:%s@github.com\n", token)
	if err := sb.PutFile(ctx, "/root/.git-credentials", []byte(line)); err != nil {
		return fmt.Errorf("write git-credentials: %w", err)
	}
	_, _ = sb.Exec(ctx, "chmod 600 /root/.git-credentials")
	return nil
}

func writeAskpass(ctx context.Context, sb core.Sandbox, token string) (path string, cleanup func(), err error) {
	path = fmt.Sprintf("/tmp/.glb_askpass_%s.sh", uuid.New().String())
	safeToken := strings.ReplaceAll(token, "'", `'\''`)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", safeToken)
	if err = sb.PutFile(ctx, path, []byte(script)); err != nil {
		return "", nil, err
	}
	if _, execErr := sb.Exec(ctx, "chmod +x "+path); execErr != nil {
		return "", nil, execErr
	}
	cleanup = func() { _, _ = sb.Exec(context.Background(), "rm -f "+path) }
	return path, cleanup, nil
}
