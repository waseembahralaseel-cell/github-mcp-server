package github

import (
	"encoding/json"
	"testing"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v74/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCommitMessage(t *testing.T) {
	tests := []struct {
		name              string
		files             []DraftCommitFile
		customInstruction string
		commitType        string
		expectedTitle     string
	}{
		{
			name: "single added file",
			files: []DraftCommitFile{
				{
					Filename:  "main.go",
					Status:    "added",
					Additions: 100,
					Deletions: 0,
				},
			},
			expectedTitle: "add main.go",
		},
		{
			name: "single modified file",
			files: []DraftCommitFile{
				{
					Filename:  "config.json",
					Status:    "modified",
					Additions: 5,
					Deletions: 2,
				},
			},
			expectedTitle: "update config.json",
		},
		{
			name: "single deleted file",
			files: []DraftCommitFile{
				{
					Filename:  "old.txt",
					Status:    "deleted",
					Additions: 0,
					Deletions: 50,
				},
			},
			expectedTitle: "remove old.txt",
		},
		{
			name: "multiple files with commit type",
			files: []DraftCommitFile{
				{
					Filename:  "feature.go",
					Status:    "added",
					Additions: 200,
				},
				{
					Filename:  "config.go",
					Status:    "modified",
					Additions: 10,
					Deletions: 5,
				},
			},
			commitType:    "feat",
			expectedTitle: "feat: update repository files",
		},
		{
			name:          "no files",
			files:         []DraftCommitFile{},
			expectedTitle: "chore: update repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := generateCommitMessage(tt.files, tt.customInstruction, tt.commitType)
			assert.Equal(t, tt.expectedTitle, message.Title)
		})
	}
}

func TestCategorizeFile(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"main.go", "source"},
		{"main_test.go", "tests"},
		{"README.md", "documentation"},
		{"config.json", "configuration"},
		{"Dockerfile", "docker"},
		{"unknown.xyz", "other"},
		{"script.py", "source"},
		{"test_utils.py", "tests"},
		{"config.yaml", "configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := categorizeFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComposeCommitMessageTool(t *testing.T) {
	// Verify tool definition
	mockClient := github.NewClient(nil)
	tool, _ := ComposeCommitMessage(stubGetClientFn(mockClient), translations.NullTranslationHelper)

	assert.Equal(t, "compose_commit_message", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "files")
	assert.Contains(t, tool.InputSchema.Properties, "custom_instruction")
	assert.Contains(t, tool.InputSchema.Properties, "commit_type")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo"})
}

func TestAnalyzeRepositoryChangesTool(t *testing.T) {
	// Verify tool definition
	mockClient := github.NewClient(nil)
	tool, _ := AnalyzeRepositoryChanges(stubGetClientFn(mockClient), translations.NullTranslationHelper)

	assert.Equal(t, "analyze_repository_changes", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "branch")
	assert.Contains(t, tool.InputSchema.Properties, "base_sha")
	assert.Contains(t, tool.InputSchema.Properties, "include_patches")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo"})
}

func TestPreviewCommitChangesTool(t *testing.T) {
	// Verify tool definition  
	mockClient := github.NewClient(nil)
	tool, _ := PreviewCommitChanges(stubGetClientFn(mockClient), translations.NullTranslationHelper)

	assert.Equal(t, "preview_commit_changes", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo") 
	assert.Contains(t, tool.InputSchema.Properties, "message")
	assert.Contains(t, tool.InputSchema.Properties, "files")
	assert.Contains(t, tool.InputSchema.Properties, "branch")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "message", "files"})
}

func TestComposeCommitsTool(t *testing.T) {
	// Verify tool definition
	mockClient := github.NewClient(nil)
	tool, _ := ComposeCommits(stubGetClientFn(mockClient), translations.NullTranslationHelper)

	assert.Equal(t, "compose_commits", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "branch")
	assert.Contains(t, tool.InputSchema.Properties, "custom_instruction")
	assert.Contains(t, tool.InputSchema.Properties, "files")
	assert.Contains(t, tool.InputSchema.Properties, "include_patches")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo"})
}

func TestCommitCompositionResponse(t *testing.T) {
	// Test the data structures
	draftCommit := DraftCommit{
		Message:     "feat: add new feature",
		Description: "This adds a new feature to the system",
		Files: []DraftCommitFile{
			{
				Filename:  "feature.go",
				Status:    "added",
				Additions: 100,
				Deletions: 0,
				Changes:   100,
			},
		},
		Stats: &MinimalCommitStats{
			Additions: 100,
			Deletions: 0,
			Total:     100,
		},
	}

	response := CommitCompositionResponse{
		DraftCommits: []DraftCommit{draftCommit},
		Summary:      "Composed 1 commit with 1 files",
		TotalFiles:   1,
		TotalChanges: 100,
	}

	// Test JSON marshaling
	data, err := json.Marshal(response)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var unmarshaled CommitCompositionResponse
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, response.Summary, unmarshaled.Summary)
	assert.Equal(t, response.TotalFiles, unmarshaled.TotalFiles)
	assert.Equal(t, response.TotalChanges, unmarshaled.TotalChanges)
	assert.Len(t, unmarshaled.DraftCommits, 1)
	assert.Equal(t, "feat: add new feature", unmarshaled.DraftCommits[0].Message)
}

func TestGenerateCompositionSummary(t *testing.T) {
	tests := []struct {
		name             string
		draftCommits     []DraftCommit
		customInstruction string
		expectedSummary  string
	}{
		{
			name:            "no commits",
			draftCommits:    []DraftCommit{},
			expectedSummary: "No commits composed",
		},
		{
			name: "single commit",
			draftCommits: []DraftCommit{
				{
					Files: []DraftCommitFile{
						{Filename: "main.go"},
						{Filename: "config.go"},
					},
				},
			},
			expectedSummary: "Composed 1 commit with 2 files",
		},
		{
			name: "multiple commits with custom instruction",
			draftCommits: []DraftCommit{
				{Files: []DraftCommitFile{{Filename: "main.go"}}},
				{Files: []DraftCommitFile{{Filename: "test.go"}}},
			},
			customInstruction: "Use conventional commits",
			expectedSummary:   "Composed 2 commits organizing 2 files following custom instructions: Use conventional commits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateCompositionSummary(tt.draftCommits, tt.customInstruction)
			assert.Equal(t, tt.expectedSummary, result)
		})
	}
}