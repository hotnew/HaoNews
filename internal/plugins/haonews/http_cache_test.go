package newsplugin

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAppFetchHTTPResponseCollapsesConcurrentColdMisses(t *testing.T) {
	t.Parallel()

	app := &App{}
	key := "api-feed:test"
	var builds int32

	build := func() (CachedHTTPResponse, error) {
		atomic.AddInt32(&builds, 1)
		time.Sleep(40 * time.Millisecond)
		return NewCachedHTTPResponse(
			200,
			"application/json; charset=utf-8",
			"private, max-age=5, stale-while-revalidate=25",
			`"feed:test"`,
			time.Now(),
			time.Now().Add(5*time.Second),
			[]byte(`{"ok":true}`),
		), nil
	}

	const callers = 12
	results := make([]CachedHTTPResponse, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = app.FetchHTTPResponse(key, build)
		}(i)
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&builds); got != 1 {
		t.Fatalf("build count = %d, want 1", got)
	}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("call %d error = %v", i, errs[i])
		}
		if string(results[i].body) != `{"ok":true}` {
			t.Fatalf("call %d body = %q", i, string(results[i].body))
		}
	}
}

func TestAppFetchHTTPResponseServesStaleWhileRefreshing(t *testing.T) {
	t.Parallel()

	app := &App{}
	key := "topic-rss:test"
	stale := NewCachedHTTPResponse(
		200,
		"application/rss+xml; charset=utf-8",
		"public, max-age=60, stale-while-revalidate=300",
		`"topic:test:stale"`,
		time.Now().Add(-2*time.Minute),
		time.Now().Add(-1*time.Second),
		[]byte("stale"),
	)
	app.storeHTTPResponse(key, stale)

	var builds int32
	builderStarted := make(chan struct{})
	releaseBuilder := make(chan struct{})
	build := func() (CachedHTTPResponse, error) {
		atomic.AddInt32(&builds, 1)
		close(builderStarted)
		<-releaseBuilder
		return NewCachedHTTPResponse(
			200,
			"application/rss+xml; charset=utf-8",
			"public, max-age=60, stale-while-revalidate=300",
			`"topic:test:fresh"`,
			time.Now(),
			time.Now().Add(60*time.Second),
			[]byte("fresh"),
		), nil
	}

	leaderDone := make(chan CachedHTTPResponse, 1)
	leaderErr := make(chan error, 1)
	go func() {
		entry, err := app.FetchHTTPResponse(key, build)
		leaderDone <- entry
		leaderErr <- err
	}()

	<-builderStarted
	start := time.Now()
	waiterEntry, waiterErr := app.FetchHTTPResponse(key, build)
	elapsed := time.Since(start)
	if waiterErr != nil {
		t.Fatalf("waiter error = %v", waiterErr)
	}
	if string(waiterEntry.body) != "stale" {
		t.Fatalf("waiter body = %q, want stale", string(waiterEntry.body))
	}
	if elapsed > 30*time.Millisecond {
		t.Fatalf("waiter took %s, expected stale response without waiting for rebuild", elapsed)
	}

	close(releaseBuilder)
	leaderEntry := <-leaderDone
	if err := <-leaderErr; err != nil {
		t.Fatalf("leader error = %v", err)
	}
	if string(leaderEntry.body) != "stale" {
		t.Fatalf("leader body = %q, want stale", string(leaderEntry.body))
	}
	if got := atomic.LoadInt32(&builds); got != 1 {
		t.Fatalf("build count = %d, want 1", got)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		fresh, ok := app.cachedHTTPResponse(key)
		if ok && string(fresh.body) == "fresh" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected background rebuild to replace stale cache")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAppFetchHTTPResponseBuilderErrorDoesNotPoisonCache(t *testing.T) {
	t.Parallel()

	app := &App{}
	key := "api-topic:test"
	var builds int32
	buildErr := errors.New("boom")

	failBuild := func() (CachedHTTPResponse, error) {
		atomic.AddInt32(&builds, 1)
		time.Sleep(20 * time.Millisecond)
		return CachedHTTPResponse{}, buildErr
	}

	const callers = 6
	start := make(chan struct{})
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = app.FetchHTTPResponse(key, failBuild)
		}(i)
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&builds); got != 1 {
		t.Fatalf("failed build count = %d, want 1", got)
	}
	for i, err := range errs {
		if !errors.Is(err, buildErr) {
			t.Fatalf("call %d err = %v, want %v", i, err, buildErr)
		}
	}
	if _, ok := app.cachedHTTPResponse(key); ok {
		t.Fatalf("expected failed build to leave cache empty")
	}

	successBuild := func() (CachedHTTPResponse, error) {
		atomic.AddInt32(&builds, 1)
		return NewCachedHTTPResponse(
			200,
			"application/json; charset=utf-8",
			"private, max-age=5, stale-while-revalidate=25",
			`"topic:test"`,
			time.Now(),
			time.Now().Add(5*time.Second),
			[]byte(`{"retry":true}`),
		), nil
	}
	entry, err := app.FetchHTTPResponse(key, successBuild)
	if err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if string(entry.body) != `{"retry":true}` {
		t.Fatalf("retry body = %q", string(entry.body))
	}
	if got := atomic.LoadInt32(&builds); got != 2 {
		t.Fatalf("total build count = %d, want 2", got)
	}
}

func TestAppFetchHTTPResponseVariantServesStaleAcrossSignatureChange(t *testing.T) {
	t.Parallel()

	app := &App{}
	key := "api-feed:page_size=20&tab=new"
	stale := NewCachedHTTPResponse(
		200,
		"application/json; charset=utf-8",
		"private, max-age=5, stale-while-revalidate=25",
		`"api-feed:old"`,
		time.Now().Add(-10*time.Second),
		time.Now().Add(-1*time.Second),
		[]byte(`{"version":"old"}`),
	)
	stale.variant = "index-old"
	app.storeHTTPResponse(key, stale)

	var builds int32
	builderStarted := make(chan struct{})
	releaseBuilder := make(chan struct{})
	build := func() (CachedHTTPResponse, error) {
		atomic.AddInt32(&builds, 1)
		close(builderStarted)
		<-releaseBuilder
		return NewCachedHTTPResponse(
			200,
			"application/json; charset=utf-8",
			"private, max-age=5, stale-while-revalidate=25",
			`"api-feed:new"`,
			time.Now(),
			time.Now().Add(5*time.Second),
			[]byte(`{"version":"new"}`),
		), nil
	}

	leaderDone := make(chan CachedHTTPResponse, 1)
	leaderErr := make(chan error, 1)
	go func() {
		entry, err := app.FetchHTTPResponseVariant(key, "index-new", build)
		leaderDone <- entry
		leaderErr <- err
	}()
	<-builderStarted

	start := time.Now()
	entry, err := app.FetchHTTPResponseVariant(key, "index-new", build)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("stale waiter error = %v", err)
	}
	if string(entry.body) != `{"version":"old"}` {
		t.Fatalf("waiter body = %q, want stale old body", string(entry.body))
	}
	if elapsed > 30*time.Millisecond {
		t.Fatalf("waiter took %s, expected stale response during variant rebuild", elapsed)
	}

	close(releaseBuilder)
	leaderEntry := <-leaderDone
	if err := <-leaderErr; err != nil {
		t.Fatalf("leader error = %v", err)
	}
	if string(leaderEntry.body) != `{"version":"old"}` {
		t.Fatalf("leader body = %q, want stale old body", string(leaderEntry.body))
	}
	if got := atomic.LoadInt32(&builds); got != 1 {
		t.Fatalf("build count = %d, want 1", got)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		cached, ok := app.cachedHTTPResponse(key)
		if ok && string(cached.body) == `{"version":"new"}` {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected background rebuild to replace stale variant cache")
		}
		time.Sleep(5 * time.Millisecond)
	}
	fresh, err := app.FetchHTTPResponseVariant(key, "index-new", build)
	if err != nil {
		t.Fatalf("fresh fetch error = %v", err)
	}
	if string(fresh.body) != `{"version":"new"}` {
		t.Fatalf("fresh body = %q, want new body", string(fresh.body))
	}
}
