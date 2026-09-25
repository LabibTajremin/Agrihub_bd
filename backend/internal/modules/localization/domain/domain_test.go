package domain

import (
	"strconv"
	"sync"
	"testing"
)

func TestValidKeyAndNamespace(t *testing.T) {
	for k, want := range map[string]bool{
		"home.quickscan.title": true, "a.b": true, "a_1.b2": true,
		"Home.title": false, "home": false, "home..x": false, "home.1x": false, "home. x": false,
	} {
		if ValidKey(k) != want {
			t.Errorf("%q", k)
		}
	}
	long := "a." + string(make([]byte, 0))
	for len(long) <= 128 {
		long += "a"
	}
	if ValidKey(long) {
		t.Fatal("overlong key")
	}
	if Namespace("home.quickscan.title") != "home" {
		t.Fatal()
	}
	if len(Errors()) != 2 {
		t.Fatal()
	}
}

func TestETag_StableAndSensitive(t *testing.T) {
	a := ETag(1, map[string]string{"x.a": "1", "x.b": "2"})
	if a != ETag(1, map[string]string{"x.b": "2", "x.a": "1"}) {
		t.Fatal("order independent")
	}
	for _, other := range []string{ETag(2, map[string]string{"x.a": "1", "x.b": "2"}), ETag(1, map[string]string{"x.a": "1", "x.b": "3"}), ETag(1, map[string]string{"x.a1": "", "x.b": "2"})} {
		if other == a {
			t.Fatal("must change with content or version")
		}
	}
	if a[0] != '"' || a[len(a)-1] != '"' {
		t.Fatal("strong etag is quoted")
	}
}

func TestCatalog_PublishAndLookup(t *testing.T) {
	var c Catalog
	if _, ok := c.Lookup("bn", "a.b"); ok {
		t.Fatal("empty catalog")
	}
	c.Publish(NewSnapshot("bn", 1, map[string]string{"a.b": "ক"}))
	c.Publish(NewSnapshot("en", 1, map[string]string{"a.b": "A"}))
	if v, ok := c.Lookup("bn", "a.b"); !ok || v != "ক" {
		t.Fatal(v)
	}
	if _, ok := c.Lookup("fr", "a.b"); ok {
		t.Fatal("missing language")
	}
	if _, ok := c.Lookup("en", "zz.z"); ok {
		t.Fatal("missing key")
	}
}

// TestCatalog_LookupIsAllocationFree asserts the O(1), zero-allocation read path.
func TestCatalog_LookupIsAllocationFree(t *testing.T) {
	var c Catalog
	c.Publish(NewSnapshot("bn", 1, map[string]string{"home.quickscan.title": "পাতা স্ক্যান করুন"}))
	if n := testing.AllocsPerRun(1000, func() { _, _ = c.Lookup("bn", "home.quickscan.title") }); n != 0 {
		t.Fatalf("lookup allocates %v times", n)
	}
}

// TestCatalog_ConcurrentReadersAndPublishers runs under -race: readers never
// block and concurrent publishers never lose an update.
func TestCatalog_ConcurrentReadersAndPublishers(t *testing.T) {
	var c Catalog
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Publish(NewSnapshot("l"+strconv.Itoa(i), int64(i), map[string]string{"k.v": "x"}))
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = c.Lookup("l0", "k.v")
			}
		}()
	}
	wg.Wait()
	for i := range 8 {
		if _, ok := c.Get("l" + strconv.Itoa(i)); !ok {
			t.Fatalf("lost update for l%d", i)
		}
	}
}

func BenchmarkCatalogLookup(b *testing.B) {
	var c Catalog
	entries := map[string]string{}
	for i := range 5000 {
		entries["screen.section.k"+strconv.Itoa(i)] = "value"
	}
	c.Publish(NewSnapshot("bn", 1, entries))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Lookup("bn", "screen.section.k4242")
		}
	})
}
