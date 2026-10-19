package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultVideoModel = "veo-3.1-generate-preview"
	maxVideoBody      = 200 << 20
	pollEvery         = 10 * time.Second
	videoWait         = 8 * time.Minute
)

// VideoRequest is one clip to make. Image, when given, is the first frame
// the clip grows from, such as the concept's render.
type VideoRequest struct {
	Prompt      string
	AspectRatio string // "9:16" or "16:9"
	Seconds     int    // 4, 6 or 8
	Resolution  string // "720p" or "1080p"
	Image       *Reference
}

type Video struct {
	MIME string
	Data []byte
}

// GenerateVideo starts a Veo generation, waits for it (generation takes
// between a few seconds and several minutes) and downloads the clip. The
// context bounds the wait.
func (c *Client) GenerateVideo(ctx context.Context, in VideoRequest) (Video, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return Video{}, &Error{Safe: "the prompt is empty"}
	}
	model := c.videoModel
	if model == "" {
		model = defaultVideoModel
	}
	inst := map[string]any{"prompt": in.Prompt}
	if in.Image != nil {
		inst["image"] = map[string]any{"inlineData": map[string]any{"mimeType": in.Image.MIME, "data": base64.StdEncoding.EncodeToString(in.Image.Data)}}
	}
	params := map[string]any{}
	if in.AspectRatio != "" {
		params["aspectRatio"] = in.AspectRatio
	}
	if in.Seconds > 0 {
		params["durationSeconds"] = fmt.Sprintf("%d", in.Seconds)
	}
	if in.Resolution != "" {
		params["resolution"] = in.Resolution
	}
	raw, err := json.Marshal(map[string]any{"instances": []map[string]any{inst}, "parameters": params})
	if err != nil {
		return Video{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/models/%s:predictLongRunning", c.base, model), bytes.NewReader(raw))
	if err != nil {
		return Video{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.key)
	res, err := c.http.Do(req)
	if err != nil {
		return Video{}, &Error{Safe: "the video service did not answer", Retryable: true, Cause: err}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	if err != nil {
		return Video{}, &Error{Safe: "the video service's answer was cut off", Retryable: true, Cause: err}
	}
	if res.StatusCode != http.StatusOK {
		e := httpError(res.StatusCode, data)
		e.Safe = strings.Replace(e.Safe, "image service", "video service", 1)
		return Video{}, e
	}
	var op struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &op); err != nil || op.Name == "" {
		return Video{}, &Error{Safe: "the video service answered in an unexpected shape", Cause: err}
	}
	uri, err := c.waitForVideo(ctx, op.Name)
	if err != nil {
		return Video{}, err
	}
	return c.download(ctx, uri)
}

func (c *Client) waitForVideo(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, videoWait)
	defer cancel()
	wait := c.poll
	if wait <= 0 {
		wait = pollEvery
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/"+name, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("x-goog-api-key", c.key)
		res, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", &Error{Safe: "the video took too long to make", Retryable: true, Cause: ctx.Err()}
			}
			return "", &Error{Safe: "the video service did not answer", Retryable: true, Cause: err}
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if err != nil {
			return "", &Error{Safe: "the video service's answer was cut off", Retryable: true, Cause: err}
		}
		if res.StatusCode != http.StatusOK {
			return "", httpError(res.StatusCode, data)
		}
		var op struct {
			Done  bool `json:"done"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Response struct {
				GenerateVideoResponse struct {
					GeneratedSamples []struct {
						Video struct {
							URI string `json:"uri"`
						} `json:"video"`
					} `json:"generatedSamples"`
					RAIMediaFilteredCount   int      `json:"raiMediaFilteredCount"`
					RAIMediaFilteredReasons []string `json:"raiMediaFilteredReasons"`
				} `json:"generateVideoResponse"`
			} `json:"response"`
		}
		if err := json.Unmarshal(data, &op); err != nil {
			return "", &Error{Safe: "the video service answered in an unexpected shape", Cause: err}
		}
		if op.Done {
			if op.Error != nil {
				return "", &Error{Safe: "the video service failed: " + clipMsg(op.Error.Message)}
			}
			r := op.Response.GenerateVideoResponse
			if len(r.GeneratedSamples) == 0 || r.GeneratedSamples[0].Video.URI == "" {
				reason := "no clip came back"
				if len(r.RAIMediaFilteredReasons) > 0 {
					reason = "the clip was filtered: " + clipMsg(r.RAIMediaFilteredReasons[0])
				}
				return "", &Error{Safe: "the video service declined (" + reason + ")"}
			}
			return r.GeneratedSamples[0].Video.URI, nil
		}
		select {
		case <-ctx.Done():
			return "", &Error{Safe: "the video took too long to make", Retryable: true, Cause: ctx.Err()}
		case <-time.After(wait):
		}
	}
}

func (c *Client) download(ctx context.Context, uri string) (Video, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return Video{}, err
	}
	req.Header.Set("x-goog-api-key", c.key)
	res, err := c.http.Do(req)
	if err != nil {
		return Video{}, &Error{Safe: "the clip could not be downloaded", Retryable: true, Cause: err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Video{}, &Error{Status: res.StatusCode, Safe: "the clip could not be downloaded", Retryable: res.StatusCode >= 500}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxVideoBody))
	if err != nil {
		return Video{}, &Error{Safe: "the clip's download was cut off", Retryable: true, Cause: err}
	}
	mime := res.Header.Get("Content-Type")
	if mime == "" || strings.HasPrefix(mime, "application/octet-stream") {
		mime = "video/mp4"
	}
	return Video{MIME: mime, Data: data}, nil
}

func clipMsg(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
