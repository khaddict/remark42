package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/remark42/backend/app/store"
	"github.com/umputun/remark42/backend/app/store/service"
)

func TestMigrator_Import(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	r := strings.NewReader(`{"version":1} {"id":"2aa0478c-df1b-46b1-b561-03d507cf482c","pid":"","text":"<p>test test #1</p>",
"user":{"name":"developer one","id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com",
"admin":true,"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},
"score":0,"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}
	{"id":"83fd97fd-ff64-48d1-9fb7-ca7769c77037","pid":"p1","text":"<p>test test #2</p>","user":{"name":"developer one",
"id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah2"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.861387771-05:00"}`)

	client := &http.Client{Timeout: 1 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native", r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	assert.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	b, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "{\"status\":\"import request accepted\"}\n", string(b))
	assert.NoError(t, resp.Body.Close())

	waitForMigrationCompletion(t, ts)

	res, code := get(t, ts.URL+"/api/v1/find?site=remark42&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments := commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&format=tree&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))
}

func TestMigrator_ImportForm(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	r := strings.NewReader(`{"version":1} {"id":"2aa0478c-df1b-46b1-b561-03d507cf482c","pid":"","text":"<p>test test #1</p>",
"user":{"name":"developer one","id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com",
"admin":true,"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},
"score":0,"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}
	{"id":"83fd97fd-ff64-48d1-9fb7-ca7769c77037","pid":"p1","text":"<p>test test #2</p>","user":{"name":"developer one",
"id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah2"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.861387771-05:00"}`)

	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)
	fileWriter, err := bodyWriter.CreateFormFile("file", "import.json")
	require.NoError(t, err)
	_, err = io.Copy(fileWriter, r)
	require.NoError(t, err)
	contentType := bodyWriter.FormDataContentType()
	require.NoError(t, bodyWriter.Close())

	authts := strings.Replace(ts.URL, "http://", "http://admin:password@", 1)
	resp, err := http.Post(authts+"/api/v1/admin/import/form?site=remark42&provider=native", contentType, bodyBuf)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	b, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "{\"status\":\"import request accepted\"}\n", string(b))
	assert.NoError(t, resp.Body.Close())

	waitForMigrationCompletion(t, ts)

	res, code := get(t, ts.URL+"/api/v1/find?site=remark42&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments := commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&format=tree&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))
}

func TestMigrator_ImportRejected(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	r := strings.NewReader(`{"version":1} {"id":"2aa0478c-df1b-46b1-b561-03d507cf482c","pid":"","text":"<p>test test #1</p>",
"user":{"name":"developer one","id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com",
"admin":true,"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},
"score":0,"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}
	{"id":"83fd97fd-ff64-48d1-9fb7-ca7769c77037","pid":"p1","text":"<p>test test #2</p>","user":{"name":"developer one",
"id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah2"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.861387771-05:00"}`)

	client := &http.Client{Timeout: 1 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native&secret=XYZ", r)
	assert.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestMigrator_ImportDouble(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	tmpl := `{"id":"%d","pid":"","text":"<p>test test #1</p>","user":{"name":"developer one","id":"dev",
"picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}`
	recs := make([]string, 0, 50)
	for i := range 50 {
		recs = append(recs, fmt.Sprintf(tmpl, i))
	}
	r := strings.NewReader(`{"version":1}` + strings.Join(recs, "\n")) // reader with 10k records
	client := &http.Client{Timeout: 1 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native", r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	assert.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	client = &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	req, err = http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native", r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	assert.NoError(t, err)
	resp, err = client.Do(req)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	waitForMigrationCompletion(t, ts)

	res, code := get(t, ts.URL+"/api/v1/find?site=remark42&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments := commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 50, comments.Info.Count)

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://radio-t.com/blah1")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 50, comments.Info.Count)
}

func TestMigrator_ImportWaitExpired(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	tmpl := `{"id":"%d","pid":"","text":"<p>test test #1</p>","user":{"name":"developer one","id":"dev",
"picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}`
	nRecs := 50
	recs := make([]string, 0, nRecs)
	for i := range nRecs {
		recs = append(recs, fmt.Sprintf(tmpl, i))
	}
	r := strings.NewReader(`{"version":1}` + strings.Join(recs, "\n")) // reader with `nRecs` records
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native", r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	require.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	client = &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/wait?site=remark42&timeout=5ms", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	assert.NoError(t, err)
	resp, err = client.Do(req)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)

	waitForMigrationCompletion(t, ts)

	res, code := get(t, ts.URL+"/api/v1/find?site=remark42&url=https://example.com/example")
	require.Equal(t, http.StatusOK, code)
	comments := commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 0, comments.Info.Count)
	require.Equal(t, 0, len(comments.Comments))
}

func TestMigrator_Export(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	r := strings.NewReader(`{"version":1} {"id":"2aa0478c-df1b-46b1-b561-03d507cf482c","pid":"","text":"<p>test test #1</p>",
"user":{"name":"developer one","id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com",
"admin":true,"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah1"},
"score":0,"votes":{},"time":"2018-04-30T01:37:00.849053725-05:00"}
	{"id":"83fd97fd-ff64-48d1-9fb7-ca7769c77037","pid":"p1","text":"<p>test test #2</p>","user":{"name":"developer one",
"id":"dev","picture":"/api/v1/avatar/remark.image","profile":"https://remark42.com","admin":true,
"ip":"ae12fe3b5f129b5cc4cdd2b136b7b7947c4d2741"},"locator":{"site":"remark42","url":"https://radio-t.com/blah2"},"score":0,
"votes":{},"time":"2018-04-30T01:37:00.861387771-05:00"}`)

	// import comments first
	client := &http.Client{Timeout: 1 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/import?site=remark42&provider=native", r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	waitForMigrationCompletion(t, ts)

	// export unknown site is a client error, not internal
	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/export?mode=file&site=test", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	errBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Contains(t, string(errBody), `"code":6`)  // rest.ErrSiteNotFound, not ErrInternal
	assert.Contains(t, string(errBody), `not found`) // error detail names the missing site

	// unknown site in stream mode is also a client error
	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/export?mode=stream&site=test", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	errBody, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(errBody), `"code":6`)

	// check file mode
	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/export?mode=file&site=remark42", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/gzip", resp.Header.Get("Content-Type"))

	ungzReader, err := gzip.NewReader(resp.Body)
	assert.NoError(t, err)
	ungzBody, err := io.ReadAll(ungzReader)
	assert.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, 3, strings.Count(string(ungzBody), "\n"))
	assert.Equal(t, 2, strings.Count(string(ungzBody), "\"text\""))
	t.Logf("%s", string(ungzBody))

	// check stream mode
	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/export?mode=stream&site=remark42", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, 3, strings.Count(string(body), "\n"))
	assert.Equal(t, 2, strings.Count(string(body), "\"text\""))
	t.Logf("%s", string(body))

	req, err = http.NewRequest("GET", ts.URL+"/api/v1/admin/export?site=remark42", http.NoBody)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestMigrator_Remap(t *testing.T) {
	ts, srv, teardown := startupT(t)
	defer teardown()

	// create 2 comments in https://remark42.com/demo/
	c1 := store.Comment{Text: "first comment", Timestamp: time.Now(),
		Locator: store.Locator{SiteID: "remark42", URL: "https://remark42.com/demo/"}, User: store.User{ID: "u1"}}
	_, err := srv.DataService.Create(c1)
	require.NoError(t, err)
	c2 := store.Comment{Text: "second comment", Timestamp: time.Now(),
		Locator: store.Locator{SiteID: "remark42", URL: "https://remark42.com/demo/"}, User: store.User{ID: "u2"}}
	_, err = srv.DataService.Create(c2)
	require.NoError(t, err)

	// create 1 comment in https://remark42.com/demo-another/
	c3 := store.Comment{Text: "third comment", Timestamp: time.Now(),
		Locator: store.Locator{SiteID: "remark42", URL: "https://remark42.com/demo-another/"}, User: store.User{ID: "u3"}}
	_, err = srv.DataService.Create(c3)
	require.NoError(t, err)

	// set url https://remark42.com/demo-another/ to be readonly
	err = srv.DataService.SetMetas("remark42", []service.UserMetaData{}, []service.PostMetaData{{
		URL:      "https://remark42.com/demo-another/",
		ReadOnly: true,
	}})
	require.NoError(t, err)

	// check that comments created as expected
	res, code := get(t, ts.URL+"/api/v1/find?site=remark42&url=https://remark42.com/demo/")
	require.Equal(t, http.StatusOK, code)
	comments := commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 2, comments.Info.Count)
	require.Equal(t, 2, len(comments.Comments))
	require.False(t, comments.Info.ReadOnly)

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://remark42.com/demo-another/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))
	require.True(t, comments.Info.ReadOnly)

	// we want remap urls to another domain - www.remark42.com
	rules := "https://remark42.com/* https://www.remark42.com/*"
	resp, err := post(t, ts.URL+"/api/v1/admin/remap?site=remark42", rules) // auth as admin
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	waitForMigrationCompletion(t, ts)

	// after remap finished we should find comments from new urls
	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://www.remark42.com/demo/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 2, comments.Info.Count)
	require.Equal(t, 2, len(comments.Comments))
	require.False(t, comments.Info.ReadOnly)

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&format=tree&url=https://www.remark42.com/demo/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 2, comments.Info.Count)
	require.Equal(t, 2, len(comments.Comments))
	require.False(t, comments.Info.ReadOnly)

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://www.remark42.com/demo-another/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))
	require.True(t, comments.Info.ReadOnly)

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&format=tree&url=https://www.remark42.com/demo-another/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 1, comments.Info.Count)
	require.Equal(t, 1, len(comments.Comments))
	require.True(t, comments.Info.ReadOnly)

	// should find nothing from previous url
	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://remark42.com/demo/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 0, comments.Info.Count)
	require.Equal(t, 0, len(comments.Comments))

	res, code = get(t, ts.URL+"/api/v1/find?site=remark42&url=https://remark42.com/demo-another/")
	require.Equal(t, http.StatusOK, code)
	comments = commentsWithInfo{}
	err = json.Unmarshal([]byte(res), &comments)
	require.NoError(t, err)
	require.Equal(t, 0, comments.Info.Count)
	require.Equal(t, 0, len(comments.Comments))
}

func TestMigrator_RemapReject(t *testing.T) {
	ts, _, teardown := startupT(t)
	defer teardown()

	// without admin credentials
	client := &http.Client{Timeout: 1 * time.Second}
	defer client.CloseIdleConnections()
	rules := strings.NewReader(`https://remark42.com/* https://www.remark42.com/*`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/admin/remap?site=remark42", rules)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func waitForMigrationCompletion(t *testing.T, ts *httptest.Server) {
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", ts.URL+"/api/v1/admin/wait?site=remark42", http.NoBody)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "{\"site_id\":\"remark42\",\"status\":\"completed\"}\n", string(b))
}
