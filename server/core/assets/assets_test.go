package assets_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/WokeWorks/adx/server/core/assets"
)

// Catches: approval following a re-render, approval being read as permission
// to deliver, or a shortcut from draft to approved.
func TestReviewIsContentAcceptanceNotDelivery(t *testing.T) {
	assert.True(t, assets.CanTransition(assets.Draft, assets.RenderedSt))
	assert.True(t, assets.CanTransition(assets.RenderedSt, assets.InReview), "publishing to the board")
	assert.True(t, assets.CanTransition(assets.InReview, assets.Approved))
	assert.False(t, assets.CanTransition(assets.Draft, assets.Approved))
	assert.False(t, assets.CanTransition(assets.Approved, assets.InReview), "approved is terminal; edits make a new version")
	assert.True(t, assets.CanTransition(assets.Revise, assets.Draft))

	v1 := assets.Ref{Key: "sha256-aaa", SHA256: "aaa", Size: 10, ContentType: "image/jpeg"}
	v2 := assets.Ref{Key: "sha256-bbb", SHA256: "bbb", Size: 11, ContentType: "image/jpeg"}
	acceptance := assets.ContentAcceptance{VersionID: 1, Ref: v1, AcceptedBy: 7}
	assert.True(t, acceptance.Covers(v1))
	assert.False(t, acceptance.Covers(v2), "a re-render is not covered by the old approval")
	assert.False(t, acceptance.GrantsDelivery())
	assert.Equal(t, [2]int{1080, 1920}, assets.Dimensions[assets.Format9x16])
}
