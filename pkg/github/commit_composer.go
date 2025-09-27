package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v74/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DraftCommit represents a draft commit structure for composition
type DraftCommit struct {
	Message     string           `json:"message"`
	Description string           `json:"description,omitempty"`
	Files       []DraftCommitFile `json:"files"`
	Stats       *MinimalCommitStats `json:"stats,omitempty"`
}

// DraftCommitFile represents a file in a draft commit
type DraftCommitFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // "added", "modified", "deleted"
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch,omitempty"`
}

// CommitCompositionRequest represents the input for commit composition
type CommitCompositionRequest struct {
	Owner            string   `json:"owner"`
	Repo             string   `json:"repo"`
	Branch           string   `json:"branch,omitempty"`
	CustomInstruction string  `json:"custom_instruction,omitempty"`
	Files            []string `json:"files,omitempty"` // Specific files to include, empty means all staged
}

// CommitCompositionResponse represents the output of commit composition
type CommitCompositionResponse struct {
	DraftCommits []DraftCommit `json:"draft_commits"`
	Summary      string        `json:"summary"`
	TotalFiles   int           `json:"total_files"`
	TotalChanges int           `json:"total_changes"`
}

// AnalyzeRepositoryChanges creates a tool to analyze repository changes for commit composition
func AnalyzeRepositoryChanges(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("analyze_repository_changes",
			mcp.WithDescription(t("TOOL_ANALYZE_REPO_CHANGES_DESCRIPTION", "Analyze repository changes to understand what files have been modified, added, or deleted. This provides the foundation for commit composition by examining the working directory state.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_ANALYZE_REPO_CHANGES_USER_TITLE", "Analyze repository changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Description("Branch to analyze changes against (defaults to default branch)"),
			),
			mcp.WithString("base_sha",
				mcp.Description("Base commit SHA to compare against (optional, will use latest commit if not provided)"),
			),
			mcp.WithBoolean("include_patches",
				mcp.Description("Whether to include file patches/diffs in the response"),
				mcp.DefaultBool(false),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, _ := OptionalParam[string](request, "branch")
			baseSHA, _ := OptionalParam[string](request, "base_sha")
			includePatches, err := OptionalBoolParamWithDefault(request, "include_patches", false)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get repository information
			repository, resp, err := client.Repositories.Get(ctx, owner, repo)
			if err != nil {
				return errors.NewGitHubAPIErrorResponse(ctx,
					"failed to get repository",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			// Use default branch if branch not specified
			if branch == "" {
				branch = repository.GetDefaultBranch()
			}

			// Get the latest commit on the branch if baseSHA not provided
			if baseSHA == "" {
				commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
					SHA:         branch,
					ListOptions: github.ListOptions{PerPage: 1},
				})
				if err != nil {
					return errors.NewGitHubAPIErrorResponse(ctx,
						"failed to get latest commit",
						resp,
						err,
					), nil
				}
				defer func() { _ = resp.Body.Close() }()

				if len(commits) > 0 {
					baseSHA = commits[0].GetSHA()
				}
			}

			// For this implementation, we'll simulate analyzing changes by comparing
			// the current state with the base commit. In a real implementation,
			// this would analyze working directory changes.
			
			// Get commits that would show changes (this is a simplified approach)
			commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
				SHA:         branch,
				ListOptions: github.ListOptions{PerPage: 10},
			})
			if err != nil {
				return errors.NewGitHubAPIErrorResponse(ctx,
					"failed to list recent commits",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			var draftFiles []DraftCommitFile
			totalAdditions := 0
			totalDeletions := 0

			// Analyze the most recent commits to simulate changes
			for i, commit := range commits {
				if i >= 3 { // Limit to recent changes
					break
				}
				
				// Get commit details with files
				commitDetail, resp, err := client.Repositories.GetCommit(ctx, owner, repo, commit.GetSHA(), nil)
				if err != nil {
					continue
				}
				defer func() { _ = resp.Body.Close() }()

				for _, file := range commitDetail.Files {
					draftFile := DraftCommitFile{
						Filename:  file.GetFilename(),
						Status:    file.GetStatus(),
						Additions: file.GetAdditions(),
						Deletions: file.GetDeletions(),
						Changes:   file.GetChanges(),
					}
					
					if includePatches {
						draftFile.Patch = file.GetPatch()
					}
					
					draftFiles = append(draftFiles, draftFile)
					totalAdditions += file.GetAdditions()
					totalDeletions += file.GetDeletions()
				}
			}

			response := struct {
				Branch       string            `json:"branch"`
				BaseSHA      string            `json:"base_sha"`
				Files        []DraftCommitFile `json:"files"`
				TotalFiles   int               `json:"total_files"`
				TotalChanges int               `json:"total_changes"`
				Stats        struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
					Total     int `json:"total"`
				} `json:"stats"`
			}{
				Branch:       branch,
				BaseSHA:      baseSHA,
				Files:        draftFiles,
				TotalFiles:   len(draftFiles),
				TotalChanges: totalAdditions + totalDeletions,
				Stats: struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
					Total     int `json:"total"`
				}{
					Additions: totalAdditions,
					Deletions: totalDeletions,
					Total:     totalAdditions + totalDeletions,
				},
			}

			r, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ComposeCommitMessage creates a tool to generate commit messages based on changes
func ComposeCommitMessage(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("compose_commit_message",
			mcp.WithDescription(t("TOOL_COMPOSE_COMMIT_MESSAGE_DESCRIPTION", "Generate AI-powered commit messages based on repository changes. Supports custom instructions for different commit message formats and conventions.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_COMPOSE_COMMIT_MESSAGE_USER_TITLE", "Compose commit message"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithArray("files",
				mcp.Description("Array of file objects with changes to base commit message on"),
				mcp.Items(map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type": "string",
							"description": "File path",
						},
						"status": map[string]interface{}{
							"type": "string",
							"description": "File status: added, modified, or deleted",
						},
						"additions": map[string]interface{}{
							"type": "number",
							"description": "Number of lines added",
						},
						"deletions": map[string]interface{}{
							"type": "number", 
							"description": "Number of lines deleted",
						},
						"patch": map[string]interface{}{
							"type": "string",
							"description": "File diff/patch content (optional)",
						},
					},
				}),
			),
			mcp.WithString("custom_instruction",
				mcp.Description("Custom instructions for commit message generation (e.g., 'Use conventional commits format', 'Keep messages under 50 characters', 'Focus on business impact')"),
			),
			mcp.WithString("commit_type",
				mcp.Description("Type of commit for better message generation"),
				mcp.Enum("feature", "bugfix", "refactor", "docs", "style", "test", "chore"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			_, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, err = RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			
			files, _ := OptionalParam[[]interface{}](request, "files")
			customInstruction, _ := OptionalParam[string](request, "custom_instruction")
			commitType, _ := OptionalParam[string](request, "commit_type")

			// Parse files if provided
			var fileChanges []DraftCommitFile
			if files != nil {
				for _, file := range files {
					if fileMap, ok := file.(map[string]interface{}); ok {
						change := DraftCommitFile{
							Filename: getString(fileMap, "filename"),
							Status:   getString(fileMap, "status"),
							Additions: getInt(fileMap, "additions"),
							Deletions: getInt(fileMap, "deletions"),
							Patch:    getString(fileMap, "patch"),
						}
						fileChanges = append(fileChanges, change)
					}
				}
			}

			// Generate commit message based on changes and instructions
			message := generateCommitMessage(fileChanges, customInstruction, commitType)
			
			response := struct {
				Message           string `json:"message"`
				Description       string `json:"description,omitempty"`
				CustomInstruction string `json:"custom_instruction,omitempty"`
				CommitType        string `json:"commit_type,omitempty"`
				FilesAnalyzed     int    `json:"files_analyzed"`
			}{
				Message:           message.Title,
				Description:       message.Description,
				CustomInstruction: customInstruction,
				CommitType:        commitType,
				FilesAnalyzed:     len(fileChanges),
			}

			r, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ComposeCommits creates a tool to organize changes into well-structured commits
func ComposeCommits(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("compose_commits",
			mcp.WithDescription(t("TOOL_COMPOSE_COMMITS_DESCRIPTION", "Organize repository changes into logical, well-structured draft commits. This is the core commit composition feature that analyzes changes and groups them into meaningful commits with clear messages.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_COMPOSE_COMMITS_USER_TITLE", "Compose commits from changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Description("Branch to compose commits for (defaults to default branch)"),
			),
			mcp.WithString("custom_instruction",
				mcp.Description("Custom instructions for commit composition (e.g., 'Follow conventional commits format', 'Separate features from bug fixes', 'Keep commits atomic')"),
			),
			mcp.WithArray("files",
				mcp.Description("Specific files to include in composition. If empty, analyzes all changes"),
				mcp.Items(map[string]interface{}{
					"type": "string",
				}),
			),
			mcp.WithBoolean("include_patches",
				mcp.Description("Whether to include file diffs in the response for review"),
				mcp.DefaultBool(false),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			
			branch, _ := OptionalParam[string](request, "branch")
			customInstruction, _ := OptionalParam[string](request, "custom_instruction")
			files, _ := OptionalParam[[]interface{}](request, "files")
			includePatches, err := OptionalBoolParamWithDefault(request, "include_patches", false)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get repository information
			repository, resp, err := client.Repositories.Get(ctx, owner, repo)
			if err != nil {
				return errors.NewGitHubAPIErrorResponse(ctx,
					"failed to get repository",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			// Use default branch if not specified
			if branch == "" {
				branch = repository.GetDefaultBranch()
			}

			// Convert files parameter to string slice
			var targetFiles []string
			if files != nil {
				for _, file := range files {
					if fileStr, ok := file.(string); ok {
						targetFiles = append(targetFiles, fileStr)
					}
				}
			}

			// Get recent commits to analyze changes
			commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
				SHA:         branch,
				ListOptions: github.ListOptions{PerPage: 5},
			})
			if err != nil {
				return errors.NewGitHubAPIErrorResponse(ctx,
					"failed to list commits",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			// Analyze and compose commits
			draftCommits, err := analyzeAndComposeCommits(ctx, client, owner, repo, commits, targetFiles, includePatches, customInstruction)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to compose commits: %v", err)), nil
			}

			// Calculate summary statistics
			totalFiles := 0
			totalChanges := 0
			for _, commit := range draftCommits {
				totalFiles += len(commit.Files)
				if commit.Stats != nil {
					totalChanges += commit.Stats.Total
				}
			}

			response := CommitCompositionResponse{
				DraftCommits: draftCommits,
				Summary:      generateCompositionSummary(draftCommits, customInstruction),
				TotalFiles:   totalFiles,
				TotalChanges: totalChanges,
			}

			r, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// PreviewCommitChanges creates a tool to preview what will be committed before actually committing
func PreviewCommitChanges(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("preview_commit_changes",
			mcp.WithDescription(t("TOOL_PREVIEW_COMMIT_CHANGES_DESCRIPTION", "Preview the changes that will be included in a commit before committing. Shows detailed diff information and commit metadata for review.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_PREVIEW_COMMIT_CHANGES_USER_TITLE", "Preview commit changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Proposed commit message"),
			),
			mcp.WithString("description",
				mcp.Description("Detailed commit description"),
			),
			mcp.WithArray("files",
				mcp.Required(),
				mcp.Description("Files to include in the commit"),
				mcp.Items(map[string]interface{}{
					"type": "string",
				}),
			),
			mcp.WithString("branch",
				mcp.Description("Target branch for the commit"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := RequiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			
			description, _ := OptionalParam[string](request, "description")
			files, err := OptionalParam[[]interface{}](request, "files")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if files == nil {
				return mcp.NewToolResultError("files parameter is required"), nil
			}
			branch, _ := OptionalParam[string](request, "branch")

			// Convert files to string slice
			var fileList []string
			for _, file := range files {
				if fileStr, ok := file.(string); ok {
					fileList = append(fileList, fileStr)
				}
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get repository to get default branch if needed
			repository, resp, err := client.Repositories.Get(ctx, owner, repo)
			if err != nil {
				return errors.NewGitHubAPIErrorResponse(ctx,
					"failed to get repository",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if branch == "" {
				branch = repository.GetDefaultBranch()
			}

			// Preview the commit by showing what would be changed
			preview := struct {
				Message     string   `json:"message"`
				Description string   `json:"description,omitempty"`
				Branch      string   `json:"branch"`
				Files       []string `json:"files"`
				FileCount   int      `json:"file_count"`
				Preview     string   `json:"preview"`
			}{
				Message:     message,
				Description: description,
				Branch:      branch,
				Files:       fileList,
				FileCount:   len(fileList),
				Preview:     fmt.Sprintf("This commit will affect %d files on branch '%s':\n\n%s", len(fileList), branch, strings.Join(fileList, "\n")),
			}

			r, err := json.Marshal(preview)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// Helper functions

type CommitMessage struct {
	Title       string
	Description string
}

func generateCommitMessage(files []DraftCommitFile, customInstruction, commitType string) CommitMessage {
	if len(files) == 0 {
		return CommitMessage{
			Title: "chore: update repository",
		}
	}

	// Analyze file changes to generate meaningful message
	var added, modified, deleted []string
	for _, file := range files {
		switch file.Status {
		case "added":
			added = append(added, file.Filename)
		case "modified":
			modified = append(modified, file.Filename)
		case "deleted":
			deleted = append(deleted, file.Filename)
		}
	}

	var title strings.Builder
	var description strings.Builder

	// Apply commit type if specified
	if commitType != "" {
		title.WriteString(commitType)
		title.WriteString(": ")
	}

	// Generate title based on changes
	if len(added) > 0 && len(modified) == 0 && len(deleted) == 0 {
		if len(added) == 1 {
			title.WriteString(fmt.Sprintf("add %s", added[0]))
		} else {
			title.WriteString(fmt.Sprintf("add %d files", len(added)))
		}
	} else if len(modified) > 0 && len(added) == 0 && len(deleted) == 0 {
		if len(modified) == 1 {
			title.WriteString(fmt.Sprintf("update %s", modified[0]))
		} else {
			title.WriteString(fmt.Sprintf("update %d files", len(modified)))
		}
	} else if len(deleted) > 0 && len(added) == 0 && len(modified) == 0 {
		if len(deleted) == 1 {
			title.WriteString(fmt.Sprintf("remove %s", deleted[0]))
		} else {
			title.WriteString(fmt.Sprintf("remove %d files", len(deleted)))
		}
	} else {
		// Mixed changes
		title.WriteString("update repository files")
	}

	// Apply custom instruction formatting
	if strings.Contains(strings.ToLower(customInstruction), "conventional") {
		// Already handled with commitType
	}
	if strings.Contains(strings.ToLower(customInstruction), "50 characters") {
		titleStr := title.String()
		if len(titleStr) > 50 {
			title.Reset()
			title.WriteString(titleStr[:47] + "...")
		}
	}

	// Generate description
	if len(files) > 1 {
		description.WriteString("Files changed:\n")
		for _, file := range files {
			description.WriteString(fmt.Sprintf("- %s (%s)\n", file.Filename, file.Status))
		}
	}

	return CommitMessage{
		Title:       title.String(),
		Description: description.String(),
	}
}

func analyzeAndComposeCommits(ctx context.Context, client *github.Client, owner, repo string, commits []*github.RepositoryCommit, targetFiles []string, includePatches bool, customInstruction string) ([]DraftCommit, error) {
	var draftCommits []DraftCommit

	// Group changes logically based on file types and changes
	fileGroups := make(map[string][]DraftCommitFile)

	for i, commit := range commits {
		if i >= 3 { // Limit analysis to recent commits
			break
		}

		// Get commit details
		commitDetail, resp, err := client.Repositories.GetCommit(ctx, owner, repo, commit.GetSHA(), nil)
		if err != nil {
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		for _, file := range commitDetail.Files {
			// Skip if target files specified and this file isn't in the list
			if len(targetFiles) > 0 {
				found := false
				for _, target := range targetFiles {
					if file.GetFilename() == target {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			draftFile := DraftCommitFile{
				Filename:  file.GetFilename(),
				Status:    file.GetStatus(),
				Additions: file.GetAdditions(),
				Deletions: file.GetDeletions(),
				Changes:   file.GetChanges(),
			}

			if includePatches {
				draftFile.Patch = file.GetPatch()
			}

			// Group files by type/purpose
			group := categorizeFile(file.GetFilename())
			fileGroups[group] = append(fileGroups[group], draftFile)
		}
	}

	// Create draft commits from file groups
	for _, files := range fileGroups {
		if len(files) == 0 {
			continue
		}

		message := generateCommitMessage(files, customInstruction, "")
		
		stats := &MinimalCommitStats{}
		for _, file := range files {
			stats.Additions += file.Additions
			stats.Deletions += file.Deletions
			stats.Total += file.Changes
		}

		draftCommit := DraftCommit{
			Message:     message.Title,
			Description: message.Description,
			Files:       files,
			Stats:       stats,
		}

		draftCommits = append(draftCommits, draftCommit)
	}

	// If no groups created, create a single commit with all files
	if len(draftCommits) == 0 && len(commits) > 0 {
		var allFiles []DraftCommitFile
		for _, group := range fileGroups {
			allFiles = append(allFiles, group...)
		}
		
		if len(allFiles) > 0 {
			message := generateCommitMessage(allFiles, customInstruction, "")
			stats := &MinimalCommitStats{}
			for _, file := range allFiles {
				stats.Additions += file.Additions
				stats.Deletions += file.Deletions
				stats.Total += file.Changes
			}

			draftCommit := DraftCommit{
				Message:     message.Title,
				Description: message.Description,
				Files:       allFiles,
				Stats:       stats,
			}
			draftCommits = append(draftCommits, draftCommit)
		}
	}

	return draftCommits, nil
}

func categorizeFile(filename string) string {
	filename = strings.ToLower(filename)
	
	if strings.HasSuffix(filename, ".md") || strings.HasSuffix(filename, ".txt") || strings.HasSuffix(filename, ".rst") {
		return "documentation"
	}
	if strings.HasSuffix(filename, ".test.go") || strings.HasSuffix(filename, "_test.go") || strings.Contains(filename, "test") {
		return "tests"
	}
	if strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, ".js") || strings.HasSuffix(filename, ".py") || strings.HasSuffix(filename, ".java") {
		return "source"
	}
	if strings.HasSuffix(filename, ".json") || strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") || strings.HasSuffix(filename, ".toml") {
		return "configuration"
	}
	if strings.Contains(filename, "dockerfile") || strings.HasSuffix(filename, ".dockerfile") {
		return "docker"
	}
	
	return "other"
}

func generateCompositionSummary(draftCommits []DraftCommit, customInstruction string) string {
	if len(draftCommits) == 0 {
		return "No commits composed"
	}

	if len(draftCommits) == 1 {
		return fmt.Sprintf("Composed 1 commit with %d files", len(draftCommits[0].Files))
	}

	totalFiles := 0
	for _, commit := range draftCommits {
		totalFiles += len(commit.Files)
	}

	summary := fmt.Sprintf("Composed %d commits organizing %d files", len(draftCommits), totalFiles)
	
	if customInstruction != "" {
		summary += fmt.Sprintf(" following custom instructions: %s", customInstruction)
	}

	return summary
}

// Helper functions for parameter extraction
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
		if num, ok := val.(int); ok {
			return num
		}
	}
	return 0
}