// Package songkey is a small client for songkey (https://github.com/MrT-coder/songkey),
// the owner's service that takes a YouTube link, skips songs already in the
// library, downloads the new ones through MeTube and files them into the
// Navidrome playlist. Only the JSON replies (format=json) are used.
//
// The token travels in the Authorization header, never in a URL.
package songkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Results of /add.
const (
	ResultQueued           = "queued"
	ResultAlreadyQueued    = "already_queued"
	ResultAlreadyHave      = "already_have" // HTTP 409, a normal outcome
	ResultPlaylistReceived = "playlist_received"
)

// Job states, in order. filed, failed and duplicate are terminal.
const (
	StateReceived   = "received"
	StateQueued     = "queued"
	StateDownloaded = "downloaded"
	StateFiled      = "filed"
	StateFailed     = "failed"
	StateDuplicate  = "duplicate"
)

// Job is one song songkey is working on.
type Job struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Reason  string `json:"reason,omitempty"`
	URL     string `json:"url,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// Terminal reports whether the job will not change any more.
func (j Job) Terminal() bool {
	return j.State == StateFiled || j.State == StateFailed || j.State == StateDuplicate
}

// Playlist is a YouTube playlist (in /add) or the library playlist (in /status).
type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AddResult is the reply of /add. Message is ready to show to a person.
type AddResult struct {
	Result   string    `json:"result"`
	Message  string    `json:"message"`
	Job      *Job      `json:"job,omitempty"`
	Playlist *Playlist `json:"playlist,omitempty"`
}

// Status is the reply of /status (without a job id).
type Status struct {
	Playlist Playlist `json:"playlist"`
	Indexed  int      `json:"indexed"`
	Active   []Job    `json:"active"`
	Recent   []Job    `json:"recent"`
}

// Error is an error reply. Error() is the server's human message.
type Error struct {
	Status     int
	Code       string // missing_url, invalid_url, unauthorized, job_not_found, too_many_attempts, ...
	Message    string
	RetryAfter int // seconds, for too_many_attempts
}

func (e *Error) Error() string {
	if e.Code == "too_many_attempts" && e.RetryAfter > 0 {
		return fmt.Sprintf("%s (try again in %d min)", e.Message, (e.RetryAfter+59)/60)
	}
	return e.Message
}

// IsCode reports whether err is a songkey error with that code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Client talks to one songkey instance.
type Client struct {
	base, token string
	http        *http.Client
}

// New returns a client for base (a trailing slash is fine).
func New(base, token string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

// Add sends a YouTube link (a song or a playlist). already_have comes back as
// a result, not as an error.
func (c *Client) Add(ctx context.Context, link string) (AddResult, error) {
	form := url.Values{"url": {link}, "format": {"json"}}
	var r AddResult
	err := c.do(ctx, http.MethodPost, "/add", strings.NewReader(form.Encode()), &r, http.StatusConflict)
	return r, err
}

// Job returns the current state of one job.
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var r struct {
		Job Job `json:"job"`
	}
	err := c.do(ctx, http.MethodGet, "/status?"+url.Values{"job": {id}, "format": {"json"}}.Encode(), nil, &r)
	return r.Job, err
}

// Status returns the library playlist and the jobs in flight.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var r Status
	err := c.do(ctx, http.MethodGet, "/status?format=json", nil, &r)
	return r, err
}

// do sends a request and decodes a JSON reply into out. Any 2xx status is a
// success, plus the ones listed in ok (409 for already_have).
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, out any, ok ...int) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	success := resp.StatusCode/100 == 2
	for _, s := range ok {
		success = success || resp.StatusCode == s
	}
	if !success {
		var e struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Error.Message == "" {
			return &Error{Status: resp.StatusCode, Code: "http", Message: "songkey answered " + resp.Status}
		}
		retry, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return &Error{Status: resp.StatusCode, Code: e.Error.Code, Message: e.Error.Message, RetryAfter: retry}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("songkey sent an unreadable reply (is the URL right?): %w", err)
	}
	return nil
}
