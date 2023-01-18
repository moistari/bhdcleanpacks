package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/moistari/bhdapi"
	"github.com/moistari/rls"
	"github.com/yookoala/realpath"
)

func main() {
	debug := flag.Bool("debug", false, "debug")
	out := flag.String("out", "2006-01-02.yml", "out")
	key := flag.String("key", os.Getenv("APIKEY"), "bhd api key")
	rss := flag.String("rss", os.Getenv("RSSKEY"), "bhd rss key")
	d := flag.Duration("d", 48*time.Hour, "duration")
	cacheDir := flag.String("cache-dir", "", "cache directory")
	flag.Parse()
	if err := run(context.Background(), *debug, *out, *key, *rss, *d, *cacheDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, debug bool, out, key, rss string, d time.Duration, cacheDir string) error {
	now := time.Now()
	cacheDir, err := getCacheDir(cacheDir)
	if err != nil {
		return err
	}
	cl := bhdapi.New(
		bhdapi.WithApiKey(key),
		bhdapi.WithRssKey(rss, false),
	)
	var torrents []bhdapi.Torrent
loop:
	for count, page, hasMore, t := 0, 1, true, now.Add(-d); hasMore; page++ {
		req := bhdapi.Search().
			WithSort("created_at").
			WithOrder("desc").
			WithAlive(true).
			WithPage(page)
		res, err := req.Do(ctx, cl)
		if err != nil {
			return err
		}
		for _, torrent := range res.Results {
			if torrent.CreatedAt.Before(t) {
				break loop
			}
			torrents = append(torrents, torrent)
			count++
		}
		hasMore = count < res.TotalResults
	}
	count := 0
	f, err := os.OpenFile(now.Format(out), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, torrent := range torrents {
		// logf("%d: %q %s -- %s", torrent.ID, torrent.Name, torrent.CreatedAt.Format("2006-01-02 15:04:05"), torrent.URL)
		meta, info, err := grabTorrent(ctx, cl, cacheDir, torrent)
		if err != nil {
			return err
		}
		if series, ok := isSeriesPack(torrent, meta, info); ok {
			logf("%s: %q %d", torrent.InfoHash[:7], torrent.Name, torrent.ID)
			if torrent.ImdbID == "" {
				logf("%s: %q %d ERROR: missing imdb id, skipping", torrent.InfoHash[:7], torrent.Name, torrent.ID)
				continue
			}
			toRemove, err := getToRemove(ctx, cl, cacheDir, torrent, series)
			if err != nil {
				return fmt.Errorf("unable to determine torrents to remove for %q %d: %w", torrent.Name, torrent.ID, err)
			}
			logf("%s: %q %d remove: %d", torrent.InfoHash[:7], torrent.Name, torrent.ID, len(toRemove))
			if n := len(toRemove); n != 0 {
				fmt.Fprintf(f, "---\nname: %q\nhash: %s\nid: %d\nurl: %s\nreplaces:\n", torrent.Name, torrent.InfoHash[:7], torrent.ID, torrent.URL)
				for _, t := range toRemove {
					fmt.Fprintf(f, "-  name: %q\n   hash: %s\n   id: %d\n   url: %s\n", t.Name, t.InfoHash[:7], t.ID, t.URL)
				}
				count += n
			}
		}
	}
	logf("remove: %d", count)
	return nil
}

func getToRemove(ctx context.Context, cl *bhdapi.Client, dir string, target bhdapi.Torrent, series int) ([]bhdapi.Torrent, error) {
	if target.ImdbID == "" {
		return nil, fmt.Errorf("missing imdb id")
	}
	var torrents []bhdapi.Torrent
	for count, page, hasMore := 0, 1, true; hasMore; page++ {
		req := bhdapi.Search().
			WithImdbID(target.ImdbID)
		res, err := req.Do(ctx, cl)
		if err != nil {
			return nil, err
		}
		for _, torrent := range res.Results {
			torrents = append(torrents, torrent)
			count++
		}
		hasMore = count < res.TotalResults
	}
	var toRemove []bhdapi.Torrent
	for _, torrent := range torrents {
		meta, info, err := grabTorrent(ctx, cl, dir, torrent)
		if err != nil {
			return nil, err
		}
		if isSingleEpOfSeries(target, torrent, meta, info, series) {
			toRemove = append(toRemove, torrent)
		}
	}
	return toRemove, nil
}

func isSeriesPack(torrent bhdapi.Torrent, meta *metainfo.MetaInfo, info *metainfo.Info) (int, bool) {
	name := info.BestName()
	r := rls.ParseString(name)
	switch {
	case !info.IsDir(), // can't be season pack, as it's only one file
		!r.Type.Is(rls.Series), // couldn't parse type, so ignore
		r.Episode != 0,
		r.Series == 0:
		return 0, false
	}
	// check actually has at least 3 sequential episodes from 1..99
	m := make(map[int]bool)
	for _, f := range info.UpvertedFiles() {
		if s := f.DisplayPath(info); strings.Contains(strings.ToLower(s), "sample") {
			continue
		}
		p := f.BestPath()
		z := rls.ParseString(p[len(p)-1])
		if z.Series == r.Series && z.Episode != 0 && validExt(z.Ext) {
			m[z.Episode] = true
		}
	}
	for i := 1; i < 99; i++ {
		sequential := true
		for j := i; j < i+3; j++ {
			if sequential = sequential && m[j]; !sequential {
				break
			}
		}
		if sequential {
			return r.Series, true
		}
	}
	return 0, false
}

func isSingleEpOfSeries(target, torrent bhdapi.Torrent, meta *metainfo.MetaInfo, info *metainfo.Info, series int) bool {
	name := info.BestName()
	r := rls.ParseString(name)
	switch {
	case !r.Type.Is(rls.Episode), // couldn't parse type, so ignore
		r.Series == 0,
		r.Episode == 0:
		return false
	}
	// check that the release doesn't have multiple series, episode tags
	tags := r.Tags()
	var sl int
	for i := len(tags) - 1; i >= 0; i-- {
		if tags[i].Is(rls.TagTypeSeries) {
			if s, _ := tags[i].Series(); s != 0 {
				sl = s
				break
			}
		}
	}
	var el int
	for i := len(tags) - 1; i >= 0; i-- {
		if tags[i].Is(rls.TagTypeSeries) {
			if _, e := tags[i].Series(); e != 0 {
				el = e
				break
			}
		}
	}
	if r.Series != sl || r.Episode != el {
		return false
	}
	m := make(map[int]bool)
	for _, f := range info.UpvertedFiles() {
		s := f.DisplayPath(info)
		if strings.Contains(strings.ToLower(s), "sample") {
			continue
		}
		if p := f.BestPath(); len(p) != 0 {
			s = p[len(p)-1]
		}
		z := rls.ParseString(s)
		if z.Series == r.Series && z.Series == series && z.Episode != 0 && validExt(z.Ext) {
			m[z.Episode] = true
		}
	}
	return len(m) == 1
}

func validExt(s string) bool {
	switch strings.ToLower(s) {
	case "mp4", "mkv", "avi":
		return true
	}
	return false
}

func grabTorrent(ctx context.Context, cl *bhdapi.Client, dir string, torrent bhdapi.Torrent) (*metainfo.MetaInfo, *metainfo.Info, error) {
	name := filepath.Join(dir, fmt.Sprintf("%d.torrent", torrent.ID))
	buf, err := getTorrent(ctx, cl, name, torrent)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read torrent for %q (%d) at %s: %w", torrent.Name, torrent.ID, name, err)
	}
	meta, err := metainfo.Load(bytes.NewReader(buf))
	if err != nil {
		return nil, nil, err
	}
	info, err := meta.UnmarshalInfo()
	if err != nil {
		return nil, nil, err
	}
	return meta, &info, nil
}

func getTorrent(ctx context.Context, cl *bhdapi.Client, name string, torrent bhdapi.Torrent) ([]byte, error) {
	if fileExists(name) {
		return os.ReadFile(name)
	}
	buf, err := cl.Torrent(ctx, torrent.ID)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(name, buf, 0o644); err != nil {
		return nil, err
	}
	return buf, nil
}

func getCacheDir(dir string) (string, error) {
	var err error
	if dir == "" {
		if dir, err = os.UserCacheDir(); err != nil {
			return "", err
		}
	}
	dir = filepath.Join(dir, "bhdcleanpacks")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return realpath.Realpath(dir)
}

func logf(s string, v ...interface{}) {
	fmt.Fprintf(os.Stdout, strings.TrimSuffix(s, "\n")+"\n", v...)
}

func fileExists(n string) bool {
	switch _, err := os.Stat(n); {
	case err == nil:
		return true
	default:
		return !os.IsNotExist(err)
	}
}
