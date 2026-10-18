package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// studioServers gives a dev server, where requests act as an admin, and a
// production one on the same data, where only public routes work.
func studioServers(t *testing.T) (dev, prod *httptest.Server, projectID int64) {
	t.Helper()
	db := storetest.Open(t)
	files, err := blobs.New(filepath.Join(t.TempDir(), "files"))
	require.NoError(t, err)
	p, err := db.Writer().CreateProject(t.Context(), store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Marlin Icon", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	deps := api.Deps{Log: slog.New(slog.DiscardHandler), Projects: projects.New(db), Studio: studio.New(db, files)}
	deps.Dev = true
	dev = httptest.NewServer(api.NewRouter(deps))
	deps.Dev = false
	prod = httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(dev.Close)
	t.Cleanup(prod.Close)
	return dev, prod, p.ID
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var v T
	require.NoError(t, json.NewDecoder(res.Body).Decode(&v))
	return v
}

func uploadJPG(t *testing.T, url string, w, h int) studio.FileRef {
	t.Helper()
	var img bytes.Buffer
	require.NoError(t, jpeg.Encode(&img, image.NewRGBA(image.Rect(0, 0, w, h)), nil))
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "ad.jpg")
	require.NoError(t, err)
	_, _ = part.Write(img.Bytes())
	require.NoError(t, mw.Close())
	res, err := http.Post(url+"/api/files", mw.FormDataContentType(), &body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	return decode[studio.FileRef](t, res)
}

// Catches: routes not reaching core, the board needing an account, files
// served without the headers that stop them running as pages, and the
// download zip missing its folders.
func TestCreativeOverHTTP(t *testing.T) {
	dev, prod, projectID := studioServers(t)

	res := postJSON(t, dev.URL+"/api/creative/projects", studio.SetupInput{ProjectID: projectID})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	settings := decode[studio.Settings](t, res)

	file := uploadJPG(t, dev.URL, 1080, 1920)
	preview := uploadJPG(t, dev.URL, 64, 114)
	assert.Equal(t, int64(1080), *file.Width)
	res = postJSON(t, fmt.Sprintf("%s/api/creative/projects/%d/uploads", dev.URL, projectID), api.UploadsRequest{Ads: []studio.UploadedAd{{
		Concept: "c1", Slug: "cash", Lang: studio.LangEN, Fmt: studio.FmtStory, File: file.SHA256, Preview: preview.SHA256,
	}}})
	require.Equal(t, http.StatusCreated, res.StatusCode, responseBody(t, res))

	// The board works outside dev, with no account.
	res, err := http.Get(prod.URL + "/api/review/" + settings.ReviewKey)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	board := decode[studio.Board](t, res)
	require.Len(t, board.Concepts, 1)
	assert.Equal(t, "C1_CASH_EN_9x16", board.Concepts[0].Ads[0].Code)

	res = postJSON(t, prod.URL+"/api/review/"+settings.ReviewKey+"/decisions", studio.BoardDecision{Ad: "C1_CASH_EN_9x16", Name: "Abdul", Status: "approved"})
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	res = postJSON(t, prod.URL+"/api/review/not-a-real-key-123/decisions", studio.BoardDecision{Ad: "C1_CASH_EN_9x16", Name: "Abdul", Status: "approved"})
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	// Everything else still needs an account outside dev.
	res, err = http.Get(fmt.Sprintf("%s/api/creative/projects/%d", prod.URL, projectID))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	res, err = http.Get(prod.URL + "/api/files/" + file.SHA256)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "image/jpeg", res.Header.Get("Content-Type"))
	assert.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
	assert.Contains(t, res.Header.Get("Content-Security-Policy"), "sandbox")
	responseBody(t, res)

	res, err = http.Get(fmt.Sprintf("%s/api/creative/projects/%d/download", dev.URL, projectID))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Disposition"), "marlinicon-approved-")
	body, _ := io.ReadAll(res.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, "C1 Cash/9x16/MARLINICON_C1_CASH_EN_9x16.jpg", zr.File[0].Name)
}

func TestUploadRejectsNonPictureOverHTTP(t *testing.T) {
	dev, _, _ := studioServers(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "page.jpg")
	_, _ = io.Copy(part, strings.NewReader("<html><script>alert(1)</script>"))
	_ = mw.Close()
	res, err := http.Post(dev.URL+"/api/files", mw.FormDataContentType(), &body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}
