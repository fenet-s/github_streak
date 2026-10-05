package githubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	githubAPIBase    = "https://api.github.com"
	githubGraphQL    = "https://api.github.com/graphql"
	rateLimitWarning = 100 // warn when remaining requests fall below this
)

// GitHubUser holds the authenticated user's profile data.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// Client wraps HTTP calls to the GitHub API.
type Client struct {
	httpClient *http.Client
}

// New creates a new GitHub API client.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// GetAuthenticatedUser fetches the profile of the user owning the given token.
func (c *Client) GetAuthenticatedUser(ctx context.Context, token string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	c.checkRateLimit(resp)

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid GitHub token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &user, nil
}

// GetContributionCount uses the GitHub GraphQL API to fetch the number of
// contributions the user made on the given date. This matches GitHub's
// contribution calendar (pushes, PRs, issues, reviews, etc.).
func (c *Client) GetContributionCount(ctx context.Context, token, username string, date time.Time) (int, error) {
	// GitHub contribution calendar uses date-only, and the from/to range is inclusive.
	dateStr := date.Format("2006-01-02")
	fromStr := dateStr + "T00:00:00Z"
	toStr := dateStr + "T23:59:59Z"

	query := fmt.Sprintf(`{
		"query": "query { user(login: \"%s\") { contributionsCollection(from: \"%s\", to: \"%s\") { contributionCalendar { totalContributions } } } }"
	}`, username, fromStr, toStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubGraphQL, bytes.NewBufferString(query))
	if err != nil {
		return 0, fmt.Errorf("creating GraphQL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("executing GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	c.checkRateLimit(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GitHub GraphQL returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the nested GraphQL response
	var result struct {
		Data struct {
			User struct {
				ContributionsCollection struct {
					ContributionCalendar struct {
						TotalContributions int `json:"totalContributions"`
					} `json:"contributionCalendar"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parsing GraphQL response: %w", err)
	}

	if len(result.Errors) > 0 {
		return 0, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	return result.Data.User.ContributionsCollection.ContributionCalendar.TotalContributions, nil
}

// checkRateLimit logs a warning when the GitHub API rate limit is running low.
func (c *Client) checkRateLimit(resp *http.Response) {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		return
	}
	n, err := strconv.Atoi(remaining)
	if err != nil {
		return
	}
	if n < rateLimitWarning {
		resetAt := resp.Header.Get("X-RateLimit-Reset")
		log.Printf("⚠️  GitHub API rate limit low: %d requests remaining (resets at %s)", n, resetAt)
	}
}
