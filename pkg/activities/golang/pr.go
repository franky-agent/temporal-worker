package golang

import (
	"context"
	"fmt"
	"os"
	"strings"

	github "github.com/google/go-github/v90/github"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	githubactivity "github.com/containifyci/temporal-worker/pkg/activities/github"
)

// CountOpenMajorUpgradePRsInputs contains parameters for counting open major upgrade PRs
type CountOpenMajorUpgradePRsInputs struct {
	Organization string
	Repository   string
}

// CountOpenMajorUpgradePRsOutputs contains the count of open major upgrade PRs
type CountOpenMajorUpgradePRsOutputs struct {
	Count int
}

// CountOpenMajorUpgradePRs counts the number of open PRs with major upgrade branches
func CountOpenMajorUpgradePRs(ctx context.Context, i CountOpenMajorUpgradePRsInputs) (CountOpenMajorUpgradePRsOutputs, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Counting open major upgrade PRs", "org", i.Organization, "repo", i.Repository)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return CountOpenMajorUpgradePRsOutputs{}, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	client := githubactivity.NewGitHubClient(githubToken)

	// List all open PRs
	allPRs := make([]*github.PullRequest, 0)
	opt := &github.PullRequestListOptions{
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		prs, res, err := client.PullRequests.List(ctx, i.Organization, i.Repository, opt)
		if err != nil {
			return CountOpenMajorUpgradePRsOutputs{}, fmt.Errorf("failed to list PRs: %w", err)
		}

		allPRs = append(allPRs, prs...)

		if res.NextPage == 0 {
			break
		}
		opt.Page = res.NextPage
	}

	// Count PRs with major upgrade branch prefix
	count := 0
	prefix := "dependabot/go_modules/major-"
	for _, pr := range allPRs {
		if pr.Head != nil && pr.Head.Ref != nil && strings.HasPrefix(*pr.Head.Ref, prefix) {
			count++
			logger.Debug("Found major upgrade PR", "prNumber", pr.GetNumber(), "branch", pr.Head.GetRef())
		}
	}

	logger.Info("Counted open major upgrade PRs", "count", count)
	return CountOpenMajorUpgradePRsOutputs{Count: count}, nil
}

// CheckPRExistsForBranchInputs contains parameters for checking if a PR exists for a branch
type CheckPRExistsForBranchInputs struct {
	Organization string
	Repository   string
	BranchName   string
}

// CheckPRExistsForBranchOutputs contains information about PR existence
type CheckPRExistsForBranchOutputs struct {
	PRURL    string
	PRNumber int
	Exists   bool
}

// CheckPRExistsForBranch checks if an open PR exists for the given branch
func CheckPRExistsForBranch(ctx context.Context, i CheckPRExistsForBranchInputs) (CheckPRExistsForBranchOutputs, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Checking if PR exists for branch", "org", i.Organization, "repo", i.Repository, "branch", i.BranchName)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return CheckPRExistsForBranchOutputs{}, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	client := githubactivity.NewGitHubClient(githubToken)

	// List open PRs and find one with matching head branch
	opt := &github.PullRequestListOptions{
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		prs, res, err := client.PullRequests.List(ctx, i.Organization, i.Repository, opt)
		if err != nil {
			return CheckPRExistsForBranchOutputs{}, fmt.Errorf("failed to list PRs: %w", err)
		}

		// Check each PR for matching branch
		for _, pr := range prs {
			if pr.Head != nil && pr.Head.Ref != nil && *pr.Head.Ref == i.BranchName {
				logger.Info("Found existing PR for branch", "prNumber", pr.GetNumber(), "branch", i.BranchName)
				return CheckPRExistsForBranchOutputs{
					Exists:   true,
					PRNumber: pr.GetNumber(),
					PRURL:    pr.GetHTMLURL(),
				}, nil
			}
		}

		if res.NextPage == 0 {
			break
		}
		opt.Page = res.NextPage
	}

	logger.Info("No existing PR found for branch", "branch", i.BranchName)
	return CheckPRExistsForBranchOutputs{Exists: false}, nil
}

// PRCreateInputs contains parameters for creating a pull request
type PRCreateInputs struct {
	Branch      string
	Description string
	Org         string
	RepoName    string
	Title       string
}

// PRCreateOutputs contains the results of creating a pull request
type PRCreateOutputs struct {
	Branch   string
	Org      string
	RepoName string
	Title    string
	Status   string
	ID       int
}

// PRCreate creates a new pull request
func PRCreate(ctx context.Context, i PRCreateInputs) (PRCreateOutputs, error) {
	logger := activity.GetLogger(ctx)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return PRCreateOutputs{}, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	client := githubactivity.NewGitHubClient(githubToken)
	sugar := zap.NewExample().Sugar()

	// Get the default branch
	accessor := NewAccessor(client, sugar)
	branch, err := accessor.GetDefaultBranch(ctx, i.RepoName, i.Org)
	if err != nil {
		return PRCreateOutputs{}, fmt.Errorf("failed to get default branch for %q: %w", i.RepoName, err)
	}

	logger.Info("default branch found", "branch", branch)

	// Prepare PR title
	title := i.Title

	// Create PR
	p, _, err := client.PullRequests.Create(ctx, i.Org, i.RepoName, github.CreatePullRequest{
		Base:  branch,
		Body:  &i.Description,
		Head:  i.Branch,
		Title: &title,
	})
	if err != nil {
		return PRCreateOutputs{}, fmt.Errorf("failed to create PR for \"%s/%s\": %w", i.Org, i.RepoName, err)
	}

	// Get PR number/ID
	prID := p.GetNumber()
	logger.Info("created PR", "pr", prID)

	out := PRCreateOutputs{
		Branch:   i.Branch,
		Org:      i.Org,
		RepoName: i.RepoName,
		ID:       prID,
		Title:    title,
		Status:   "created",
	}

	return out, nil
}

// PRAddLabelsInputs contains parameters for adding labels to a PR
type PRAddLabelsInputs struct {
	Organization string
	Repository   string
	Labels       []string
	PRNumber     int
}

// PRAddLabels adds labels to a pull request
func PRAddLabels(ctx context.Context, i PRAddLabelsInputs) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Adding labels to PR", "pr", i.PRNumber, "labels", i.Labels)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	client := githubactivity.NewGitHubClient(githubToken)
	sugar := zap.NewExample().Sugar()
	accessor := NewAccessor(client, sugar)

	// Add labels to the PR (PRs are treated as issues in GitHub API)
	_, _, err := accessor.IssuesAddLabels(ctx, i.Organization, i.Repository, i.PRNumber, i.Labels)
	if err != nil {
		return fmt.Errorf("failed to add labels to PR#%d: %w", i.PRNumber, err)
	}

	logger.Info("Labels added successfully", "pr", i.PRNumber)
	return nil
}

// PRCommentInputs contains parameters for commenting on a PR
type PRCommentInputs struct {
	Message  string
	Org      string
	RepoName string
	ID       int
}

// PRComment adds a new comment to an existing pull request
func PRComment(ctx context.Context, i PRCommentInputs) (string, error) {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return "", fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	client := githubactivity.NewGitHubClient(githubToken)

	_, _, err := client.Issues.CreateComment(ctx, i.Org, i.RepoName, i.ID, &github.IssueComment{
		Body: &i.Message,
	})
	if err != nil {
		return "", fmt.Errorf("failed to add comment to PR#%d: %w", i.ID, err)
	}

	return "PR Commented", nil
}

// Accessor wraps the GitHub client with helper methods.
// We provide a minimal implementation here since we don't depend on go-githubapp's Accessor.
type Accessor struct {
	client *github.Client
	logger *zap.SugaredLogger
}

// NewAccessor creates a new Accessor
func NewAccessor(client *github.Client, logger *zap.SugaredLogger) *Accessor {
	return &Accessor{client: client, logger: logger}
}

// GetDefaultBranch returns the default branch for a repository
func (a *Accessor) GetDefaultBranch(ctx context.Context, repo, org string) (string, error) {
	r, _, err := a.client.Repositories.Get(ctx, org, repo)
	if err != nil {
		return "", err
	}
	if r.DefaultBranch != nil {
		return *r.DefaultBranch, nil
	}
	return "main", nil
}

// IssuesAddLabels adds labels to an issue/PR
func (a *Accessor) IssuesAddLabels(ctx context.Context, org, repo string, number int, labels []string) ([]*github.Label, *github.Response, error) {
	return a.client.Issues.AddLabelsToIssue(ctx, org, repo, number, labels)
}


