package studio

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/runs"
)

type fakeMotion struct{ specs []MotionSpec }

func (f *fakeMotion) Build(_ context.Context, m MotionSpec) ([]byte, []byte, error) {
	f.specs = append(f.specs, m)
	// A minimal MP4 header the sniffer recognises, and a tiny JPEG poster.
	mp4 := append([]byte{0, 0, 0, 24}, []byte("ftypisom")...)
	mp4 = append(mp4, make([]byte, 64)...)
	return mp4, jpgBytes(360, 640, 7), nil
}

func (f *fakeMotion) Poster(_ context.Context, _ []byte) ([]byte, error) {
	return jpgBytes(360, 640, 9), nil
}

type fakeVideo struct {
	calls []ClipRequest
	fail  error
}

func (f *fakeVideo) GenerateVideo(_ context.Context, in ClipRequest) (Clip, error) {
	f.calls = append(f.calls, in)
	if f.fail != nil {
		return Clip{}, f.fail
	}
	mp4 := append([]byte{0, 0, 0, 24}, []byte("ftypisom")...)
	return Clip{MIME: "video/mp4", Data: append(mp4, make([]byte, 64)...)}, nil
}

func jpgBytes(w, h int, shade uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func (f *fixture) conceptID(t *testing.T, code string) int64 {
	t.Helper()
	p, err := f.c.Project(t.Context(), viewer, f.project)
	require.NoError(t, err)
	for _, k := range p.Concepts {
		if k.Code == code {
			return k.ID
		}
	}
	t.Fatalf("no concept %s", code)
	return 0
}

func (f *fixture) work(t *testing.T, rs *runs.Runs, run runs.Run) runs.Run {
	t.Helper()
	h, err := rs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	completion, execErr := f.c.Execute(t.Context(), h)
	if execErr != nil {
		require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StateFailed, Completeness: runs.CompletenessPartial, Error: execErr.Error()}))
	} else {
		require.NoError(t, h.Finish(t.Context(), completion))
	}
	got, err := rs.Get(t.Context(), coretest.Viewer, run.ID)
	require.NoError(t, err)
	return got
}

// Catches: a video ad named like a picture, the finished story ad not
// ending the motion, the same ask starting two runs, and a model clip kept
// without a poster.
func TestVideoAdsAreMadeByRuns(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	coretest.SeedUsers(t, f.db)
	rs := runs.New(f.db, nil, runs.Config{})
	ws := core.Actor{Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	f.uploadStory(10) // C1 CASH EN 9:16, the card the motion ends on
	k := f.conceptID(t, "C1")
	_, err := f.c.AddRenders(ctx, marketer, f.project, []NewRender{{File: f.jpg(1080, 1920, 20), Preview: f.jpg(270, 480, 21), Label: "Pool"}})
	require.NoError(t, err)
	motion := &fakeMotion{}
	video := &fakeVideo{}
	f.c.Generate(Generation{Motion: motion, Video: video})

	_, err = f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: "dance"})
	assert.Error(t, err)

	started, err := f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: VideoMotion})
	require.NoError(t, err)
	assert.True(t, started.Started)
	assert.Equal(t, "video", started.Ad.Kind)
	assert.Equal(t, "C1_CASH_VIDEO_EN_9x16", started.Ad.Code)
	assert.Equal(t, "MARLINICON_C1_CASH_VIDEO_EN_9x16.mp4", started.Ad.FileName)
	assert.Nil(t, started.Ad.Current, "nothing until the run finishes")
	again, err := f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: VideoMotion})
	require.NoError(t, err)
	assert.False(t, again.Started, "the same version is made once")

	done := f.work(t, rs, started.Run)
	assert.Equal(t, runs.StateSucceeded, done.State, done.Error)
	require.Len(t, motion.specs, 1)
	assert.Len(t, motion.specs[0].Pictures, 1, "the project's render moves")
	assert.NotNil(t, motion.specs[0].Card, "and the finished story ad closes it")
	p, err := f.c.Project(ctx, viewer, f.project)
	require.NoError(t, err)
	var vid *Ad
	for _, a := range p.Concepts[0].Ads {
		if a.Kind == "video" {
			vid = &a
		}
	}
	require.NotNil(t, vid)
	require.NotNil(t, vid.Current)
	assert.Equal(t, "video/mp4", vid.Current.File.ContentType)
	assert.Equal(t, "image/jpeg", vid.Current.Preview.ContentType)
	assert.Equal(t, VideoMotion, vid.Current.Method)
	assert.Equal(t, StatusWaiting, vid.Status, "reviewed like any ad")

	// A model clip: the render goes in as the first frame, a poster is taken.
	_, err = f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: VideoModel})
	assert.Error(t, err, "no imagery prompt on the concept yet")
	modelRun, err := f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: VideoModel, Prompt: "Dusk over the pool"})
	require.NoError(t, err)
	assert.True(t, modelRun.Started, "a new version of the same ad")
	done = f.work(t, rs, modelRun.Run)
	assert.Equal(t, runs.StateSucceeded, done.State, done.Error)
	require.Len(t, video.calls, 1)
	assert.Contains(t, video.calls[0].Prompt, "Dusk over the pool")
	assert.Equal(t, 8, video.calls[0].Seconds)
	require.NotNil(t, video.calls[0].Image)
	p, err = f.c.Project(ctx, viewer, f.project)
	require.NoError(t, err)
	for _, a := range p.Concepts[0].Ads {
		if a.Kind == "video" {
			assert.Equal(t, int64(2), a.Current.Version)
			assert.Equal(t, VideoModel, a.Current.Method)
		}
	}

	// An outage fails the run with a message the person may read.
	video.fail = &PictureError{Safe: "the video service is having trouble", Retryable: true}
	r3, err := f.c.StartVideo(ctx, ws, rs, k, VideoInput{Lang: LangEN, Mode: VideoModel, Prompt: "x"})
	require.NoError(t, err)
	failed := f.work(t, rs, r3.Run)
	assert.Equal(t, runs.StateFailed, failed.State)
	assert.Contains(t, *failed.Error, "having trouble")
}
