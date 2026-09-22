package jira

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// JiraIssuePattern is the regex pattern for matching Jira issue keys.
// Matches format: PROJECT-1234 (e.g., RED-123, TEC-456, DEVOPS-789)
var JiraIssuePattern = regexp.MustCompile(`\b([A-Z]+-[0-9]+)\b`)

// IssueInfo contains information about a Jira issue extracted from a component.
type IssueInfo struct {
	Component string `json:"component"`
	Issue     string `json:"issue"`
	URL       string `json:"url"`
	Version   string `json:"version"`
}

// ParseIssueFromString extracts a Jira issue number from a string.
// Returns the first Jira issue found in format PROJECT-123, or empty string if not found.
func ParseIssueFromString(text string) string {
	if text == "" {
		return ""
	}

	matches := JiraIssuePattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// ParseAllIssuesFromString extracts all Jira issue numbers from a string.
// Returns a slice of all Jira issues found in format PROJECT-123.
func ParseAllIssuesFromString(text string) []string {
	if text == "" {
		return nil
	}

	matches := JiraIssuePattern.FindAllStringSubmatch(text, -1)
	var issues []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			issue := match[1]
			if !seen[issue] {
				issues = append(issues, issue)
				seen[issue] = true
			}
		}
	}

	return issues
}

// ValidateIssueFormat checks if a string matches the Jira issue format.
func ValidateIssueFormat(issueKey string) bool {
	return JiraIssuePattern.MatchString(issueKey)
}

// SaveIssuesToFile saves a list of Jira issues to a JSON file.
func SaveIssuesToFile(issues []IssueInfo, saveDir, saveFile string) (string, error) {
	if saveDir == "" {
		return "", errors.NewParamError("save directory must be specified")
	}
	if saveFile == "" {
		saveFile = "jira.json"
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to create save directory: %s", saveDir)
	}

	// Build full file path
	filePath := filepath.Join(saveDir, saveFile)

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(issues, "", "  ")
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to marshal Jira issues to JSON")
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to write Jira issues to file: %s", filePath)
	}

	logging.Debug("Saved %d Jira issues to %s", len(issues), filePath)

	return filePath, nil
}

// LoadIssuesFromFile loads Jira issues from a JSON file.
func LoadIssuesFromFile(filePath string) ([]IssueInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to read Jira issues file: %s", filePath)
	}

	var issues []IssueInfo
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to parse Jira issues JSON")
	}

	return issues, nil
}

// ExtractIssueFromVersion extracts Jira issue from a version string.
// Common patterns:
// - "feature/RED-123-new-feature" -> "RED-123"
// - "RED-456" -> "RED-456"
// - "v1.2.3-TEC-789" -> "TEC-789"
func ExtractIssueFromVersion(version string) string {
	return ParseIssueFromString(version)
}

// ExtractIssueFromURL extracts Jira issue from a URL or path.
// Common patterns:
// - "https://github.com/org/repo/tree/RED-123-branch" -> "RED-123"
// - "git@github.com:org/TEC-456.git" -> "TEC-456"
func ExtractIssueFromURL(url string) string {
	return ParseIssueFromString(url)
}

// FormatIssueURL formats a Jira issue key into a Jira URL.
// Requires the Jira base URL to be configured.
func FormatIssueURL(issueKey, jiraBaseURL string) string {
	if jiraBaseURL == "" {
		return issueKey
	}

	// Ensure base URL doesn't have trailing slash
	jiraBaseURL = strings.TrimSuffix(jiraBaseURL, "/")

	return fmt.Sprintf("%s/browse/%s", jiraBaseURL, issueKey)
}
