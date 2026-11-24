package songkey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fake answers like songkey does, and records what it was sent.
func fake(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"code":"unauthorized","message":"wrong or missing token"}}`)
			return
		}
		if r.URL.Query().Get("token") != "" {
			t.Error("the token must travel in the header, never in the URL")
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", "tok") // a trailing slash in the config must not matter
}

func TestAddQueuedSendsLinkAsJSONRequestAndReturnsTheJob(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/add" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		if r.PostForm.Get("url") != "https://youtu.be/Cr8K88UcO5s" || r.PostForm.Get("format") != "json" {
			t.Errorf("form = %v", r.PostForm)
		}
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"result":"queued","message":"QUEUED\nTití","job":{"id":"3fa91c","state":"received","title":"Tití Me Preguntó","artist":""},"notify":true}`)
	})
	r, err := c.Add(context.Background(), "https://youtu.be/Cr8K88UcO5s")
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != ResultQueued || r.Job == nil || r.Job.ID != "3fa91c" || r.Job.State != StateReceived || r.Message != "QUEUED\nTití" {
		t.Fatalf("got %+v", r)
	}
}

func TestAddAlreadyHaveIsAnOutcomeNotAnError(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"result":"already_have","message":"ALREADY HAVE IT","track":{"title":"Tití"}}`)
	})
	r, err := c.Add(context.Background(), "https://youtu.be/x")
	if err != nil || r.Result != ResultAlreadyHave || r.Message != "ALREADY HAVE IT" || r.Job != nil {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestAddPlaylistReceivedCarriesThePlaylist(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"result":"playlist_received","message":"PLAYLIST RECEIVED","playlist":{"id":"PL1","name":"Un Verano"}}`)
	})
	r, err := c.Add(context.Background(), "https://youtube.com/playlist?list=PL1")
	if err != nil || r.Result != ResultPlaylistReceived || r.Playlist == nil || r.Playlist.Name != "Un Verano" {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestErrorsKeepTheirCodeAndMessage(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"code":"invalid_url","message":"could not read a YouTube video id from that link"}}`)
	})
	_, err := c.Add(context.Background(), "nope")
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_url" || e.Status != 400 || err.Error() != "could not read a YouTube video id from that link" {
		t.Fatalf("got %#v", err)
	}
}

func TestWrongTokenIsUnauthorized(t *testing.T) {
	c := fake(t, nil)
	c.token = "bad"
	_, err := c.Add(context.Background(), "https://youtu.be/x")
	var e *Error
	if !errors.As(err, &e) || e.Code != "unauthorized" {
		t.Fatalf("got %v", err)
	}
}

func TestTooManyAttemptsReportsRetryAfter(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "900")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"code":"too_many_attempts","message":"too many wrong tokens"}}`)
	})
	_, err := c.Job(context.Background(), "3fa91c")
	var e *Error
	if !errors.As(err, &e) || e.Code != "too_many_attempts" || e.RetryAfter != 900 || !strings.Contains(err.Error(), "15 min") {
		t.Fatalf("got %v", err)
	}
}

func TestNonJSONReplyIsAReadableError(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "<html>Bad gateway</html>")
	})
	_, err := c.Add(context.Background(), "https://youtu.be/x")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("got %v", err)
	}
}

func TestJobAndStatus(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" || r.URL.Query().Get("format") != "json" {
			t.Errorf("got %s", r.URL)
		}
		switch r.URL.Query().Get("job") {
		case "3fa91c":
			io.WriteString(w, `{"job":{"id":"3fa91c","state":"failed","title":"T","reason":"MeTube said no"}}`)
		case "":
			io.WriteString(w, `{"playlist":{"id":"pl-9","name":"DETODO"},"indexed":3,"active":[],"recent":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":{"code":"job_not_found","message":"no job with that id"}}`)
		}
	})
	j, err := c.Job(context.Background(), "3fa91c")
	if err != nil || j.State != StateFailed || j.Reason != "MeTube said no" || !j.Terminal() {
		t.Fatalf("job: %+v %v", j, err)
	}
	st, err := c.Status(context.Background())
	if err != nil || st.Playlist.ID != "pl-9" || st.Playlist.Name != "DETODO" {
		t.Fatalf("status: %+v %v", st, err)
	}
	_, err = c.Job(context.Background(), "zzz")
	if !IsCode(err, "job_not_found") {
		t.Fatalf("missing job: %v", err)
	}
}

func TestTerminalStates(t *testing.T) {
	for st, want := range map[string]bool{StateReceived: false, StateQueued: false, StateDownloaded: false, StateFiled: true, StateFailed: true, StateDuplicate: true} {
		if (Job{State: st}).Terminal() != want {
			t.Errorf("%s: want terminal=%v", st, want)
		}
	}
}
