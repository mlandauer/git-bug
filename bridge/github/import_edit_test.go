package github

import (
	"context"
	"testing"

	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/repository/repotest"
)

// An edit from Github must find what it edits even when other operations
// carry the same Github id: exporting a title, status or label change, or
// an edition of the issue body, marks the operation with the issue's id, and
// exporting a comment edition marks it with the comment's.
func TestImportEditAfterExport(t *testing.T) {
	ctx := context.Background()
	const t0 = 1700000000
	repo := repotest.NewGoGitRepo(t, false)
	backend, err := cache.NewRepoCacheNoEvents(repo, nil)
	require.NoError(t, err)
	defer backend.Close()

	author, err := backend.Identities().New(ctx, "author", "author@example.com")
	require.NoError(t, err)
	require.NoError(t, backend.SetUserIdentity(ctx, author))

	// an issue imported from Github, with a comment
	b, _, err := backend.Bugs().NewRaw(author, t0, "title", "body", nil, map[string]string{metaKeyGithubId: "ISSUE"})
	require.NoError(t, err)
	commentId, _, err := b.AddCommentRaw(author, t0+1, "comment", nil, map[string]string{metaKeyGithubId: "COMMENT"})
	require.NoError(t, err)

	// changes made locally, then exported: marked as the exporter does
	title, err := b.SetTitleRaw(author, t0+2, "new title", nil)
	require.NoError(t, err)
	closed, err := b.CloseRaw(author, t0+3, nil)
	require.NoError(t, err)
	_, bodyEdit, err := b.EditCreateCommentRaw(author, t0+4, "body edited here", nil)
	require.NoError(t, err)
	commentEdit, err := b.EditCommentRaw(author, t0+5, commentId, "comment edited here", nil)
	require.NoError(t, err)
	require.NoError(t, b.Commit(ctx))
	for id, ghId := range map[string]string{
		string(title.Id()): "ISSUE", string(closed.Id()): "ISSUE", string(bodyEdit.Id()): "ISSUE",
		string(commentEdit.Id()): "COMMENT",
	} {
		_, err := b.SetMetadata(ctx, entity.Id(id), map[string]string{metaKeyGithubId: ghId})
		require.NoError(t, err)
	}
	require.NoError(t, b.Commit(ctx))

	// then edits made on Github are imported
	gi := &githubImporter{out: make(chan core.ImportResult, 10)}
	body := githubv4.String("body edited on Github")
	err = gi.ensureIssueEdit(ctx, backend, b, githubv4.ID("ISSUE"), &userContentEdit{
		Id: githubv4.ID("BODY_EDIT"), Editor: &actor{Typename: "User", Login: "someone"}, Diff: &body})
	require.NoError(t, err)
	comment := githubv4.String("comment edited on Github")
	err = gi.ensureCommentEdit(ctx, backend, b, githubv4.ID("COMMENT"), &userContentEdit{
		Id: githubv4.ID("COMMENT_EDIT"), Editor: &actor{Typename: "User", Login: "someone"}, Diff: &comment})
	require.NoError(t, err)

	snap := b.Snapshot()
	require.Equal(t, "body edited on Github", snap.Comments[0].Message)
	require.Equal(t, "comment edited on Github", snap.Comments[1].Message)
}
