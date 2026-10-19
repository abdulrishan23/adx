package google

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Catches: the key sent in the URL instead of the header, the aspect ratio
// not reaching the request, a blocked prompt reported as a picture, and an
// outage reported as a refusal.
func TestGenerateImageRequestAndResponses(t *testing.T) {
	var got map[string]any
	var gotKey, gotPath string
	mode := "ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-goog-api-key")
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		switch mode {
		case "blocked":
			_, _ = w.Write([]byte(`{"promptFeedback":{"blockReason":"SAFETY"},"candidates":[]}`))
		case "down":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
		default:
			_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString([]byte("PNGBYTES")) + `"}}]}}]}`))
		}
	}))
	defer srv.Close()
	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	require.NoError(t, err)

	img, err := c.GenerateImage(t.Context(), ImageRequest{Prompt: "a villa at dusk", AspectRatio: "9:16", Size: "1K", References: []Reference{{MIME: "image/jpeg", Data: []byte("JPG")}}})
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.MIME)
	assert.Equal(t, []byte("PNGBYTES"), img.Data)
	assert.Equal(t, "k", gotKey)
	assert.Equal(t, "/models/gemini-3.1-flash-image:generateContent", gotPath)
	gen := got["generationConfig"].(map[string]any)
	assert.Equal(t, "9:16", gen["imageConfig"].(map[string]any)["aspectRatio"])
	assert.Equal(t, []any{"IMAGE"}, gen["responseModalities"])
	parts := got["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2, "the reference picture goes before the prompt")
	assert.Equal(t, "a villa at dusk", parts[1].(map[string]any)["text"])

	mode = "blocked"
	_, err = c.GenerateImage(t.Context(), ImageRequest{Prompt: "x"})
	var ge *Error
	require.ErrorAs(t, err, &ge)
	assert.False(t, ge.Retryable)
	assert.Contains(t, ge.Safe, "declined")

	mode = "down"
	_, err = c.GenerateImage(t.Context(), ImageRequest{Prompt: "x"})
	require.ErrorAs(t, err, &ge)
	assert.True(t, ge.Retryable)
}

// Catches: the clip reported before the operation is done, a filtered clip
// reported as a clip, and the download sent without the key.
func TestGenerateVideoPollsAndDownloads(t *testing.T) {
	polls := 0
	var startBody map[string]any
	var downloadKey string
	mode := "ok"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":predictLongRunning"):
			require.NoError(t, json.NewDecoder(r.Body).Decode(&startBody))
			_, _ = w.Write([]byte(`{"name":"models/veo/operations/abc"}`))
		case strings.HasPrefix(r.URL.Path, "/models/veo/operations/"):
			polls++
			if polls < 2 {
				_, _ = w.Write([]byte(`{"done":false}`))
				return
			}
			if mode == "filtered" {
				_, _ = w.Write([]byte(`{"done":true,"response":{"generateVideoResponse":{"generatedSamples":[],"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["people"]}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"` + srv.URL + `/download/clip"}}]}}}`))
		case r.URL.Path == "/download/clip":
			downloadKey = r.Header.Get("x-goog-api-key")
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("MP4BYTES"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, err := New(Config{APIKey: "k", BaseURL: srv.URL, Poll: time.Millisecond})
	require.NoError(t, err)
	v, err := c.GenerateVideo(t.Context(), VideoRequest{Prompt: "slow push in over the lagoon", AspectRatio: "9:16", Seconds: 8, Resolution: "720p", Image: &Reference{MIME: "image/jpeg", Data: []byte("JPG")}})
	require.NoError(t, err)
	assert.Equal(t, []byte("MP4BYTES"), v.Data)
	assert.Equal(t, "video/mp4", v.MIME)
	assert.Equal(t, "k", downloadKey)
	assert.Equal(t, 2, polls)
	params := startBody["parameters"].(map[string]any)
	assert.Equal(t, "9:16", params["aspectRatio"])
	assert.Equal(t, "8", params["durationSeconds"])
	inst := startBody["instances"].([]any)[0].(map[string]any)
	assert.NotNil(t, inst["image"])

	polls, mode = 0, "filtered"
	_, err = c.GenerateVideo(t.Context(), VideoRequest{Prompt: "x"})
	var ge *Error
	require.ErrorAs(t, err, &ge)
	assert.Contains(t, ge.Safe, "filtered")
}
