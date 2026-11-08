package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"searchtags/internal/store"
)

// fakeStore подменяет ClickHouse. Ключи data — уже нормализованные запросы.
type fakeStore struct {
	data  map[string][]store.Tag
	err   error
	calls []string
}

func (f *fakeStore) Tags(_ context.Context, q string) ([]store.Tag, error) {
	f.calls = append(f.calls, q)
	if f.err != nil {
		return nil, f.err
	}
	return f.data[q], nil
}

// blockingStore висит, пока не истечёт контекст (проверка таймаута).
type blockingStore struct{}

func (blockingStore) Tags(ctx context.Context, _ string) ([]store.Tag, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func newTestServer(s store.TagStore) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(s, log, time.Second).Handler()
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func apiURL(q string) string { return "/api/tags?q=" + url.QueryEscape(q) }

func TestAPIEmptyQueryReturns400(t *testing.T) {
	h := newTestServer(&fakeStore{})
	for _, q := range []string{"", "   ", "\t\u00a0"} {
		if rec := get(h, apiURL(q)); rec.Code != http.StatusBadRequest {
			t.Errorf("q=%q: status = %d, want 400", q, rec.Code)
		}
	}
	// параметра q нет вообще
	if rec := get(h, "/api/tags"); rec.Code != http.StatusBadRequest {
		t.Errorf("no q: status = %d, want 400", rec.Code)
	}
}

func TestAPIDoesNotHitStoreOnBadQuery(t *testing.T) {
	fs := &fakeStore{}
	get(newTestServer(fs), apiURL("  "))
	if len(fs.calls) != 0 {
		t.Errorf("store was called %d times, want 0", len(fs.calls))
	}
}

func TestAPINormalizationGivesSameAnswer(t *testing.T) {
	fs := &fakeStore{data: map[string][]store.Tag{
		"платье": {{Tag: "красное платье", Users: 12}, {Tag: "вечернее платье", Users: 9}},
	}}
	h := newTestServer(fs)

	a := get(h, apiURL("Платье "))
	b := get(h, apiURL("платье"))
	c := get(h, apiURL("  ПЛАТЬЕ\t"))

	if a.Code != 200 || b.Code != 200 || c.Code != 200 {
		t.Fatalf("statuses = %d, %d, %d, want all 200", a.Code, b.Code, c.Code)
	}
	if a.Body.String() != b.Body.String() || b.Body.String() != c.Body.String() {
		t.Errorf("bodies differ:\n%s\n%s\n%s", a.Body, b.Body, c.Body)
	}
	for _, q := range fs.calls {
		if q != "платье" {
			t.Errorf("store got %q, want normalized %q", q, "платье")
		}
	}
}

func TestAPIReturnsTagsInOrder(t *testing.T) {
	fs := &fakeStore{data: map[string][]store.Tag{
		"платье": {{Tag: "красное платье", Users: 12}, {Tag: "вечернее платье", Users: 9}},
	}}
	rec := get(newTestServer(fs), apiURL("платье"))

	var resp tagsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(resp.Tags) != 2 || resp.Tags[0].Tag != "красное платье" || resp.Tags[1].Users != 9 {
		t.Errorf("unexpected tags: %+v", resp.Tags)
	}
}

func TestAPINoTagsReturnsEmptyListAnd200(t *testing.T) {
	rec := get(newTestServer(&fakeStore{}), apiURL("несуществующий запрос"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"tags":[]`) {
		t.Errorf("body = %s, want empty list (not null)", rec.Body)
	}
}

func TestAPIStoreErrorReturns500(t *testing.T) {
	rec := get(newTestServer(&fakeStore{err: io.ErrUnexpectedEOF}), apiURL("платье"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestAPITimeoutReturns504(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(blockingStore{}, log, 20*time.Millisecond).Handler()
	if rec := get(h, apiURL("платье")); rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", rec.Code)
	}
}

func TestPageShowsEmptyMessage(t *testing.T) {
	rec := get(newTestServer(&fakeStore{}), "/?q="+url.QueryEscape("нет такого"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Для этого запроса пока нет тегов.") {
		t.Errorf("status=%d, body missing empty-state message", rec.Code)
	}
}

func TestPageTagIsClickableLink(t *testing.T) {
	fs := &fakeStore{data: map[string][]store.Tag{"платье": {{Tag: "красное платье", Users: 5}}}}
	rec := get(newTestServer(fs), "/?q="+url.QueryEscape("платье"))
	want := "/?q=%d0%ba%d1%80%d0%b0%d1%81%d0%bd%d0%be%d0%b5%20%d0%bf%d0%bb%d0%b0%d1%82%d1%8c%d0%b5"
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("link %q not found in body:\n%s", want, rec.Body)
	}
}

func TestPageEscapesUserInputAndTags(t *testing.T) {
	evil := `<script>alert(1)</script>`
	fs := &fakeStore{data: map[string][]store.Tag{
		"x": {{Tag: `<b onmouseover="x">tag</b>`, Users: 5}},
	}}
	h := newTestServer(fs)

	// ввод пользователя
	body := get(h, "/?q="+url.QueryEscape(evil)).Body.String()
	if strings.Contains(body, evil) {
		t.Errorf("raw user input leaked into HTML")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("escaped user input not found")
	}

	// теги из базы тоже содержат пользовательский текст
	body = get(h, "/?q=x").Body.String()
	if strings.Contains(body, `<b onmouseover`) {
		t.Errorf("raw tag leaked into HTML")
	}
}

func TestPageEmptyQueryReturns400(t *testing.T) {
	if rec := get(newTestServer(&fakeStore{}), "/?q="); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPageWithoutQueryIsOK(t *testing.T) {
	rec := get(newTestServer(&fakeStore{}), "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `name="q"`) {
		t.Errorf("status=%d, search form not found", rec.Code)
	}
}
