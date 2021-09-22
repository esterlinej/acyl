package ghclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v38/github"
)

// GitHubAppInstallationClient describes a GitHub client that returns user-scoped metadata regarding an app installation
type GitHubAppInstallationClient interface {
	GetUserAppInstallations(ctx context.Context) (AppInstallations, error)
	GetUserAppRepos(ctx context.Context, appID int64) ([]string, error)
	GetUser(ctx context.Context) (string, error)
	GetUserAppRepoPermissions(ctx context.Context, instID int64) (map[string]AppRepoPermissions, error)
	GetInstallationTokenForRepo(ctx context.Context, instID int64, reponame string) (string, error)
}

type AppInstallation struct {
	ID int64
}

type AppInstallations []AppInstallation

func (ai AppInstallations) IDPresent(id int64) bool {
	for _, inst := range ai {
		if inst.ID == id {
			return true
		}
	}
	return false
}

func appInstallationsFromGitHubInstallations(in []*github.Installation) AppInstallations {
	out := make(AppInstallations, len(in))
	for i, inst := range in {
		if inst != nil {
			if inst.ID != nil {
				out[i].ID = *inst.ID
			}
		}
	}
	return out
}

// GetUserAppInstallationCount returns the number of app installations that are accessible to the authenticated user
// This method only uses the static token associated with the GitHubClient and not anything present in the context
// GitHubClient should be populated with the user token returned by the oauth login endpoint via the oauth callback handler
func (ghc *GitHubClient) GetUserAppInstallations(ctx context.Context) (AppInstallations, error) {
	lopt := &github.ListOptions{PerPage: 100}
	out := []*github.Installation{}
	for {
		ctx, cf := context.WithTimeout(ctx, ghTimeout)
		defer cf()
		insts, resp, err := ghc.c.Apps.ListUserInstallations(ctx, lopt)
		if err != nil {
			return nil, fmt.Errorf("error listing user installations: %v", err)
		}
		out = append(out, insts...)
		if resp.NextPage == 0 {
			return appInstallationsFromGitHubInstallations(out), nil
		}
		lopt.Page = resp.NextPage
	}
}

// GetUserAppRepos gets repositories that are accessible to the authenticated user for an app installation
// This method only uses the static token associated with the GitHubClient and not anything present in the context
// GitHubClient should be populated with the user token returned by the oauth login endpoint via the oauth callback handler
func (ghc *GitHubClient) GetUserAppRepos(ctx context.Context, instID int64) ([]string, error) {
	lopt := &github.ListOptions{PerPage: 100}
	out := []string{}
	for {
		ctx, cf := context.WithTimeout(ctx, ghTimeout)
		defer cf()
		repos, resp, err := ghc.c.Apps.ListUserRepos(ctx, instID, lopt)
		if err != nil {
			return nil, fmt.Errorf("error listing user repos: %v", err)
		}
		for _, repo := range repos.Repositories {
			if repo != nil && repo.FullName != nil {
				out = append(out, *repo.FullName)
			}
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		lopt.Page = resp.NextPage
	}
}

// GetUser gets the authenticated user login name
// This method only uses the static token associated with the GitHubClient and not anything present in the context
// GitHubClient should be populated with the user token returned by the oauth login endpoint via the oauth callback handler
func (ghc *GitHubClient) GetUser(ctx context.Context) (string, error) {
	ctx, cf := context.WithTimeout(ctx, ghTimeout)
	defer cf()
	user, _, err := ghc.c.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("error getting current authenticated user: %v", err)
	}
	return user.GetLogin(), nil
}

type AppRepoPermissions struct {
	Repo              string
	Admin, Push, Pull bool
}

func (ghc *GitHubClient) GetUserAppRepoPermissions(ctx context.Context, instID int64) (map[string]AppRepoPermissions, error) {
	lopt := &github.ListOptions{PerPage: 100}
	rs := []*github.Repository{}
	for {
		ctx, cf := context.WithTimeout(ctx, ghTimeout)
		defer cf()
		repos, resp, err := ghc.c.Apps.ListUserRepos(ctx, instID, lopt)
		if err != nil {
			return nil, fmt.Errorf("error listing user repos: %v", err)
		}
		rs = append(rs, repos.Repositories...)
		if resp.NextPage == 0 {
			break
		}
		lopt.Page = resp.NextPage
	}
	out := make(map[string]AppRepoPermissions, len(rs))
	for _, r := range rs {
		fn := r.GetFullName()
		p := r.GetPermissions()
		out[fn] = AppRepoPermissions{
			Repo:  fn,
			Admin: p["admin"],
			Push:  p["push"],
			Pull:  p["pull"],
		}
	}
	return out, nil
}

// GetInstallationTokenForRepo gets a repo-scoped GitHub access token with read permissions for repo with a validity period of one hour,
// for use with subsequent GitHub API calls by external systems (Furan).
// This app installation must have access to repo or the call will return an error.
func (ghc *GitHubClient) GetInstallationTokenForRepo(ctx context.Context, instID int64, reponame string) (string, error) {
	// get repo id
	rs := strings.SplitN(reponame, "/", 1)
	if len(rs) != 2 {
		return "", fmt.Errorf("malformed repo name (expected: [owner]/[name]): %v", reponame)
	}
	c := ghc.getClient(ctx)
	repo, _, err := c.Repositories.Get(ctx, rs[0], rs[1])
	if err != nil || repo == nil || repo.ID == nil {
		return "", fmt.Errorf("error getting repo details: %w", err)
	}
	read := "read"
	tkn, _, err := c.Apps.CreateInstallationToken(ctx, instID, &github.InstallationTokenOptions{
		RepositoryIDs: []int64{*repo.ID},
		Permissions: &github.InstallationPermissions{
			Contents:          &read,
			ContentReferences: &read,
			Metadata:          &read,
		},
	})
	if err != nil || tkn == nil || tkn.Token == nil {
		return "", fmt.Errorf("error getting installation token: %w", err)
	}
	return *tkn.Token, nil
}
