package goodmem_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
)

func TestDownloadHeadersArePerRequest(t *testing.T) {
	for _, page := range []bool{false, true} {
		t.Run(strconv.FormatBool(page), func(t *testing.T) {
			calls := 0
			headers := map[string]string{"rAnGe": "bytes=1-2", "Accept-Encoding": "identity", "If-Match": `"v1"`}
			c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "gm_test" {
					t.Error("missing authentication")
				}
				if page {
					if r.URL.Path != "/v1/memories/m1/pages/2/image" || r.URL.Query().Get("dpi") != "300" {
						t.Errorf("wrong page URL: %s", r.URL)
					}
				} else if r.URL.Path != "/v1/memories/m1/content" {
					t.Errorf("wrong content URL: %s", r.URL)
				}
				if calls == 0 {
					for name, want := range headers {
						if r.Header.Get(name) != want {
							t.Errorf("%s: %q", name, r.Header.Get(name))
						}
					}
					w.WriteHeader(http.StatusPartialContent)
				} else if r.Header.Get("Range") != "" || r.Header.Get("If-Match") != "" || r.Header.Get("Accept-Encoding") == "identity" {
					t.Error("download headers leaked to next call")
				}
				calls++
				_, _ = w.Write([]byte{0, 255})
			})
			var data []byte
			var err error
			dpi := int32(300)
			params := &goodmem.MemoriesPagesImageParams{Dpi: &dpi}
			if page {
				data, err = c.Memories().PagesImageWithHeaders(context.Background(), "m1", "2", params, headers)
			} else {
				data, err = c.Memories().ContentWithHeaders(context.Background(), "m1", headers)
			}
			if err != nil || string(data) != string([]byte{0, 255}) {
				t.Fatalf("data %v, error %v", data, err)
			}
			if page {
				_, err = c.Memories().PagesImage(context.Background(), "m1", "2", params)
			} else {
				_, err = c.Memories().Content(context.Background(), "m1")
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(headers) != 3 || headers["rAnGe"] != "bytes=1-2" {
				t.Fatal("caller headers mutated")
			}
		})
	}
}

func TestDownloadHeadersPreserveHTTPFailures(t *testing.T) {
	for _, status := range []int{412, 416} {
		for _, page := range []bool{false, true} {
			t.Run(strconv.Itoa(status)+"/"+strconv.FormatBool(page), func(t *testing.T) {
				c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte("download rejected"))
				})
				var err error
				if page {
					_, err = c.Memories().PagesImageWithHeaders(context.Background(), "m1", "0", nil, nil)
				} else {
					_, err = c.Memories().ContentWithHeaders(context.Background(), "m1", nil)
				}
				var apiErr *goodmem.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status || apiErr.Body != "download rejected" {
					t.Fatalf("wrong error: %v", err)
				}
			})
		}
	}
}
