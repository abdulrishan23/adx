package google

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
