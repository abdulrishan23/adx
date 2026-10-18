package studio

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	marketer = core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	viewer   = core.Actor{UserID: 0, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	public   = core.Anonymous(core.ViaWeb)
)

type fixture struct {
	t       *testing.T
	c       *Studio
	db      *store.DB
	project int64
	clock   time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := storetest.Open(t)
	files, err := blobs.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, c: New(db, files), db: db, clock: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	f.c.now = func() time.Time { return f.clock }
	p, err := db.Writer().CreateProject(t.Context(), store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Marlin Icon", CreatedAt: f.clock})
	if err != nil {
		t.Fatal(err)
	}
	f.project = p.ID
	if _, err := f.c.SetUp(t.Context(), marketer, SetupInput{ProjectID: p.ID}); err != nil {
		t.Fatal(err)
	}
	return f
}

// jpg uploads a JPG of the given size. shade makes each picture's bytes
// different, since identical uploads are stored once.
func (f *fixture) jpg(w, h int, shade uint8) string {
	f.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		f.t.Fatal(err)
	}
	ref, err := f.c.Upload(f.t.Context(), marketer, &buf)
	if err != nil {
		f.t.Fatal(err)
	}
	return ref.SHA256
}

func (f *fixture) uploadStory(shade uint8) UploadResult {
	f.t.Helper()
	res, err := f.c.AddUploads(f.t.Context(), marketer, f.project, []UploadedAd{{
		Concept: "C1", Slug: "CASH", Lang: LangEN, Fmt: FmtStory,
		File: f.jpg(1080, 1920, shade), Preview: f.jpg(64, 114, shade),
	}})
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) onlyAd() Ad {
	f.t.Helper()
	p, err := f.c.Project(f.t.Context(), viewer, f.project)
	if err != nil {
		f.t.Fatal(err)
	}
	if len(p.Concepts) != 1 || len(p.Concepts[0].Ads) != 1 {
		f.t.Fatalf("want one concept with one ad, got %+v", p.Concepts)
	}
	return p.Concepts[0].Ads[0]
}

func (f *fixture) key() string {
	f.t.Helper()
	p, err := f.c.Project(f.t.Context(), viewer, f.project)
	if err != nil {
		f.t.Fatal(err)
	}
	return p.Settings.ReviewKey
}

func (f *fixture) decide(name, status string) {
	f.t.Helper()
	if err := f.c.Decide(f.t.Context(), public, f.key(), BoardDecision{Ad: "C1_CASH_EN_9x16", Name: name, Status: status}); err != nil {
		f.t.Fatal(err)
	}
	f.clock = f.clock.Add(time.Second)
}

func wantKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Kind != kind {
		t.Fatalf("got %v, want a core error of kind %d", err, kind)
	}
}

// Uploading names the ad the way Creative Studio did and creates its concept.
func TestUploadCreatesConceptAndNamesAd(t *testing.T) {
	f := setup(t)
	res := f.uploadStory(10)
	if res != (UploadResult{Concepts: 1, Ads: 1, Versions: 1}) {
		t.Fatalf("got %+v", res)
	}
	ad := f.onlyAd()
	if ad.Code != "C1_CASH_EN_9x16" || ad.FileName != "MARLINICON_C1_CASH_EN_9x16.jpg" {
		t.Fatalf("named %s / %s", ad.Code, ad.FileName)
	}
	if ad.Status != StatusWaiting || ad.Current == nil || ad.Current.Version != 1 {
		t.Fatalf("got status %s, current %+v", ad.Status, ad.Current)
	}
}

// A picture that isn't exactly its size's pixels would reach the board and
// the ad platforms under the wrong name.
func TestUploadRejectsWrongSize(t *testing.T) {
	f := setup(t)
	_, err := f.c.AddUploads(t.Context(), marketer, f.project, []UploadedAd{{
		Concept: "C1", Slug: "CASH", Lang: LangEN, Fmt: FmtStory,
		File: f.jpg(1080, 1350, 1), Preview: f.jpg(64, 80, 1),
	}})
	wantKind(t, err, core.KindInvalid)
}

// Files are served publicly, so a page or script must never be stored as one.
func TestUploadRejectsNonPictures(t *testing.T) {
	f := setup(t)
	_, err := f.c.Upload(t.Context(), marketer, strings.NewReader("<html><script>alert(1)</script></html>"))
	wantKind(t, err, core.KindInvalid)
}

// The core rule of the port: an approval belongs to the picture it was
// given on. A new version must start waiting again.
func TestNewVersionResetsReview(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	f.decide("Abdul", "approved")
	if s := f.onlyAd().Status; s != StatusApproved {
		t.Fatalf("after approval got %s", s)
	}
	f.uploadStory(20) // same ad, new picture
	ad := f.onlyAd()
	if ad.Status != StatusWaiting || ad.Current.Version != 2 || len(ad.Decisions) != 0 {
		t.Fatalf("after a new version got %s v%d with %d decisions", ad.Status, ad.Current.Version, len(ad.Decisions))
	}
}

func TestReviseOutranksApprovalsAndLatestPerNameCounts(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	f.decide("Abdul", "approved")
	f.decide("Sara", "revise")
	if s := f.onlyAd().Status; s != StatusRevise {
		t.Fatalf("one revise among approvals gave %s", s)
	}
	f.decide("sara", "approved") // same reviewer, different case
	ad := f.onlyAd()
	if ad.Status != StatusApproved || len(ad.Decisions) != 2 {
		t.Fatalf("got %s with %+v", ad.Status, ad.Decisions)
	}
	f.decide("Abdul", "clear")
	f.decide("Sara", "clear")
	if s := f.onlyAd().Status; s != StatusWaiting {
		t.Fatalf("after both cleared got %s", s)
	}
}

// A paused or replaced link must stop working for reading and posting.
func TestPausedAndRotatedLinksStopWorking(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	old := f.key()
	if _, err := f.c.Board(t.Context(), public, old); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.RotateReviewKey(t.Context(), marketer, f.project); err != nil {
		t.Fatal(err)
	}
	_, err := f.c.Board(t.Context(), public, old)
	wantKind(t, err, core.KindNotFound)

	s, err := f.c.Project(t.Context(), viewer, f.project)
	if err != nil {
		t.Fatal(err)
	}
	in := SettingsInput{Prefix: s.Settings.Prefix, PageName: s.Settings.PageName, ReviewActive: false}
	if _, err := f.c.UpdateSettings(t.Context(), marketer, f.project, in); err != nil {
		t.Fatal(err)
	}
	err = f.c.Decide(t.Context(), public, s.Settings.ReviewKey, BoardDecision{Ad: "C1_CASH_EN_9x16", Name: "A", Status: "approved"})
	wantKind(t, err, core.KindNotFound)
}

func TestBoardShowsOnlyPublishedAdsAndPinnedComments(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	concept := f.mustConcept()
	if _, err := f.c.SaveGenerated(t.Context(), marketer, concept, GeneratedAd{
		Fmt: FmtFeed, Lang: LangEN, Slug: "CASH", Design: json.RawMessage(`{"v":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	x, y := 0.5, 0.25
	if _, err := f.c.Comment(t.Context(), public, f.key(), CommentInput{Ad: "C1_CASH_EN_9x16", Name: "Abdul", Body: " Logo too small ", X: &x, Y: &y}); err != nil {
		t.Fatal(err)
	}
	b, err := f.c.Board(t.Context(), public, f.key())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Concepts) != 1 || len(b.Concepts[0].Ads) != 1 {
		t.Fatalf("the unpublished 4:5 layout reached the board: %+v", b.Concepts)
	}
	if len(b.Comments) != 1 || b.Comments[0].Body != "Logo too small" || *b.Comments[0].Y != 0.25 {
		t.Fatalf("got comments %+v", b.Comments)
	}
	half := 0.5
	_, err = f.c.Comment(t.Context(), public, f.key(), CommentInput{Ad: "C1_CASH_EN_9x16", Name: "A", Body: "x", X: &half})
	wantKind(t, err, core.KindInvalid)
}

// An open link must not let one person flood a board.
func TestCommentsAreRateLimited(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	key := f.key()
	for i := range maxCommentsPerMinute {
		if _, err := f.c.Comment(t.Context(), public, key, CommentInput{Ad: "C1_CASH_EN_9x16", Name: "A", Body: "ok"}); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
	}
	_, err := f.c.Comment(t.Context(), public, key, CommentInput{Ad: "C1_CASH_EN_9x16", Name: "A", Body: "one too many"})
	wantKind(t, err, core.KindConflict)
	f.clock = f.clock.Add(2 * time.Minute)
	if _, err := f.c.Comment(t.Context(), public, key, CommentInput{Ad: "C1_CASH_EN_9x16", Name: "A", Body: "later"}); err != nil {
		t.Fatalf("a minute later: %v", err)
	}
}

func TestPublishingADraftMakesAVersion(t *testing.T) {
	f := setup(t)
	concept := f.mustConcept()
	gen, err := f.c.SaveGenerated(t.Context(), marketer, concept, GeneratedAd{
		Fmt: FmtSquare, Lang: LangAR, CTA: true, Slug: "CASH", RenderCode: "R01", Design: json.RawMessage(`{"v":1,"layers":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gen.HadPicture || gen.Ad.Status != StatusUnpublished || gen.Ad.Code != "C1_CASH_AR_1x1_CTA" {
		t.Fatalf("got %+v", gen)
	}
	ad, err := f.c.Publish(t.Context(), marketer, gen.Ad.ID, PublishInput{
		File: f.jpg(1080, 1080, 5), Preview: f.jpg(64, 64, 5), Design: json.RawMessage(`{"v":1,"layers":[1]}`), Source: "generated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ad.Status != StatusWaiting || ad.Current.Source != "generated" {
		t.Fatalf("got %+v", ad)
	}
	detail, err := f.c.Ad(t.Context(), viewer, ad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(detail.Design) != `{"v":1,"layers":[1]}` || string(detail.Draft) != string(detail.Design) {
		t.Fatalf("design %s, draft %s", detail.Design, detail.Draft)
	}
	again, err := f.c.SaveGenerated(t.Context(), marketer, concept, GeneratedAd{
		Fmt: FmtSquare, Lang: LangAR, CTA: true, Slug: "CASH", Design: json.RawMessage(`{"v":1}`),
	})
	if err != nil || !again.HadPicture {
		t.Fatalf("regenerating an ad with a picture: %+v, %v", again, err)
	}
}

func TestVariationIsNamedAfterItsRender(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	if _, err := f.c.AddRenders(t.Context(), marketer, f.project, []NewRender{{File: f.jpg(300, 200, 7), Preview: f.jpg(30, 20, 7), Label: "Canal"}}); err != nil {
		t.Fatal(err)
	}
	ad, err := f.c.CreateVariation(t.Context(), marketer, f.onlyAdID(), VariationInput{RenderCode: "R01", Design: json.RawMessage(`{"v":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if ad.Code != "C1_CASH_R01_EN_9x16" || ad.RenderCode != "R01" || ad.Status != StatusUnpublished {
		t.Fatalf("got %+v", ad)
	}
}

// Ads keep their file names, so a later prefix would split one download
// across two naming schemes.
func TestPrefixIsFixedOnceAdsExist(t *testing.T) {
	f := setup(t)
	in := SettingsInput{Prefix: "MARLIN", PageName: "Reportage Properties", ReviewActive: true}
	if _, err := f.c.UpdateSettings(t.Context(), marketer, f.project, in); err != nil {
		t.Fatal(err)
	}
	f.uploadStory(10)
	in.Prefix = "MI"
	_, err := f.c.UpdateSettings(t.Context(), marketer, f.project, in)
	wantKind(t, err, core.KindConflict)
}

func TestDownloadApprovedOnly(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	_, entries, err := f.c.Download(t.Context(), viewer, f.project, true)
	if err != nil || len(entries) != 0 {
		t.Fatalf("an unreviewed ad was in the approved set: %v %v", entries, err)
	}
	f.decide("Abdul", "approved")
	name, entries, err := f.c.Download(t.Context(), viewer, f.project, true)
	if err != nil {
		t.Fatal(err)
	}
	if name != "marlinicon-approved-2026-10-17.zip" || len(entries) != 1 ||
		entries[0].Path != "C1 Cash/9x16/MARLINICON_C1_CASH_EN_9x16.jpg" {
		t.Fatalf("got %s %+v", name, entries)
	}
}

func TestRolesAreEnforced(t *testing.T) {
	f := setup(t)
	_, err := f.c.CreateConcept(t.Context(), viewer, f.project, NewConcept{Code: "C2"})
	wantKind(t, err, core.KindForbidden)
	_, err = f.c.Project(t.Context(), public, f.project)
	wantKind(t, err, core.KindForbidden)
	_, err = f.c.Upload(t.Context(), viewer, strings.NewReader("x"))
	wantKind(t, err, core.KindForbidden)
}

func TestPickIgnoresHiddenRenders(t *testing.T) {
	f := setup(t)
	f.uploadStory(10)
	renders, err := f.c.AddRenders(t.Context(), marketer, f.project, []NewRender{{File: f.jpg(300, 200, 7), Preview: f.jpg(30, 20, 7)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.c.PickRender(t.Context(), public, f.key(), PickInput{Concept: "C1", Render: "R01", Name: "Abdul"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.UpdateRender(t.Context(), marketer, renders[0].ID, RenderInput{Hidden: true}); err != nil {
		t.Fatal(err)
	}
	err = f.c.PickRender(t.Context(), public, f.key(), PickInput{Concept: "C1", Render: "R01", Name: "Abdul"})
	wantKind(t, err, core.KindNotFound)
}

func (f *fixture) mustConcept() int64 {
	f.t.Helper()
	p, err := f.c.Project(f.t.Context(), viewer, f.project)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, k := range p.Concepts {
		if k.Code == "C1" {
			return k.ID
		}
	}
	k, err := f.c.CreateConcept(f.t.Context(), marketer, f.project, NewConcept{Code: "C1", Name: "Cash"})
	if err != nil {
		f.t.Fatal(err)
	}
	return k.ID
}

func (f *fixture) onlyAdID() int64 { return f.onlyAd().ID }
