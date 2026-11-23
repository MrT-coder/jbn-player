package subsonic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadReturnsOriginalFileAndExtension(t *testing.T) {
	cases := []struct {
		name, disp, ctype, wantExt string
	}{
		{"disposition wins", `attachment; filename="01 - song.FLAC"`, "audio/mpeg", "flac"},
		{"content type fallback", "", "audio/mpeg", "mp3"},
		{"unknown", "", "application/octet-stream", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotID string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotID = r.URL.Path, r.URL.Query().Get("id")
				if tc.disp != "" {
					w.Header().Set("Content-Disposition", tc.disp)
				}
				w.Header().Set("Content-Type", tc.ctype)
				_, _ = w.Write([]byte("audio-bytes"))
			}))
			defer srv.Close()

			rc, ext, err := New(srv.URL, "u", "p").Download(context.Background(), "song42")
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			body, _ := io.ReadAll(rc)
			if string(body) != "audio-bytes" || ext != tc.wantExt {
				t.Fatalf("body=%q ext=%q, want ext %q", body, ext, tc.wantExt)
			}
			if gotPath != "/rest/download.view" || gotID != "song42" {
				t.Fatalf("request = %s id=%s", gotPath, gotID)
			}
		})
	}
}

func TestDownloadFailsOnHTTPErrorAndSubsonicError(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"http 500": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) },
		// Subsonic reports failures as a JSON body with status 200.
		"json error": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"failed","error":{"code":70,"message":"not found"}}}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			if rc, _, err := New(srv.URL, "u", "p").Download(context.Background(), "x"); err == nil {
				rc.Close()
				t.Fatal("expected an error")
			}
		})
	}
}
