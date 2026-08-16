package github

import (
	"net/http"

	github "github.com/google/go-github/v90/github"
	"golang.org/x/oauth2"
)

// NewGitHubClient creates an authenticated GitHub client using a personal access token
func NewGitHubClient(token string) *github.Client {
	client, err := github.NewClient(github.WithAuthToken(token))
	if err != nil {
		// WithAuthToken should never return an error, but handle it defensively
		panic("unexpected error creating GitHub client: " + err.Error())
	}
	return client
}

// NewGitHubClientWithHTTP creates an authenticated GitHub client with a custom HTTP client
func NewGitHubClientWithHTTP(token string, httpClient *http.Client) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	// Create a transport that wraps the provided HTTP client's transport with OAuth2
	transport := &oauth2.Transport{
		Source: ts,
		Base:   httpClient.Transport,
	}
	oauthClient := &http.Client{
		Transport: transport,
		// Copy relevant fields from the provided client
		CheckRedirect: httpClient.CheckRedirect,
		Timeout:       httpClient.Timeout,
		Jar:           httpClient.Jar,
	}
	client, err := github.NewClient(github.WithHTTPClient(oauthClient))
	if err != nil {
		panic("unexpected error creating GitHub client: " + err.Error())
	}
	return client
}