package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func artifactScope(scope string) string { page, _, _ := strings.Cut(scope, "/"); return page }
func (e *engine) loadArtifacts() error {
	file := filepath.Join(e.dir, "artifacts", "index.json")
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("invalid artifact index")
	}
	var index map[string]artifact
	if json.Unmarshal(raw, &index) != nil {
		return errors.New("invalid artifact index")
	}
	for id, a := range index {
		if !strings.HasPrefix(id, "a_") || !idPattern.MatchString(id) || len(a.Scope) > 256 || a.Size < 0 || a.Size > 4<<20 {
			return errors.New("invalid artifact index")
		}
		path := filepath.Join(e.dir, "artifacts", id)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == a.Size {
			e.artifacts[id] = a
		}
	}
	return nil
}
func (e *engine) storeArtifact(owner, kind string, data []byte) (string, error) {
	if len(data) > 4<<20 {
		return "", errors.New("artifact_too_large")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	total := int64(len(data))
	for _, a := range e.artifacts {
		total += a.Size
	}
	if len(e.artifacts) >= 1024 || total > 192<<20 {
		return "", errors.New("artifact_limit")
	}
	dir := filepath.Join(e.dir, "artifacts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if actual, err := filepath.EvalSymlinks(dir); err != nil || actual != dir {
		return "", errors.New("storage_failed")
	}
	id := "a_" + rand.Text()
	path := filepath.Join(dir, id)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	e.artifacts[id] = artifact{Scope: artifactScope(owner), Kind: kind, Size: int64(len(data))}
	raw, err := json.Marshal(e.artifacts)
	temp := filepath.Join(dir, ".index-"+rand.Text())
	if err == nil {
		err = os.WriteFile(temp, raw, 0600)
	}
	if err == nil {
		err = os.Rename(temp, filepath.Join(dir, "index.json"))
	}
	os.Remove(temp)
	if err != nil {
		delete(e.artifacts, id)
		os.Remove(path)
		return "", err
	}
	return id, nil
}

//go:embed snapshot.js
var snapshotTemplate string
var snapshotSource = withSensitivePolicy(snapshotTemplate)

func (e *engine) snapshot(owner string, p *rod.Page, t *target) (string, error) {
	ctx, cancel := context.WithTimeout(p.GetContext(), 4*time.Second)
	defer cancel()
	p = p.Context(ctx)
	// Save a bounded structural DOM only. Never serialize input values,
	// text, attributes, HTML, URLs or a cross-origin frame's contents.
	shape, err := p.Eval(snapshotSource)
	if err != nil {
		return "", err
	}
	if shape.Value.Get("unsafe").Bool() {
		return "", errors.New("sensitive_snapshot")
	}
	raw, err := json.Marshal(shape.Value.Get("nodes").Val())
	if err != nil {
		return "", err
	}
	domID, err := e.storeArtifact(owner, "application/json", raw)
	if err != nil {
		return "", err
	}
	quality := 45
	var data []byte
	if t != nil && len(t.Locators) > 0 {
		findCtx, stop := context.WithTimeout(ctx, 700*time.Millisecond)
		el, findErr := findTarget(findCtx, p, *t)
		if findErr == nil {
			data, _ = passiveElementScreenshot(el.Page().Context(findCtx), el.Object.ObjectID, quality)
		}
		stop()
	}
	var thumbnail []byte
	metrics, metricErr := (proto.PageGetLayoutMetrics{}).Call(p)
	if metricErr == nil && metrics.CSSContentSize != nil {
		size := metrics.CSSContentSize
		width, height := math.Max(1, math.Min(size.Width, 16000)), math.Max(1, math.Min(size.Height, 16000))
		shot, shotErr := (proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatJpeg, Quality: &quality, FromSurface: true, CaptureBeyondViewport: true, Clip: &proto.PageViewport{Width: width, Height: height, Scale: math.Min(.25, 1600/math.Max(width, height))}}).Call(p)
		if shotErr == nil {
			thumbnail = shot.Data
		}
	}
	if len(data) == 0 {
		data, err = p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatJpeg, Quality: &quality})
	}
	if err != nil || len(data) == 0 {
		return "", err
	}
	id, err := e.storeArtifact(owner, "image/jpeg", data)
	if err != nil {
		return "", err
	}
	thumbID := ""
	if len(thumbnail) > 0 {
		thumbID, _ = e.storeArtifact(owner, "image/jpeg", thumbnail)
	}
	e.mu.Lock()
	a := e.artifacts[id]
	a.Thumbnail = thumbID
	a.DOM = domID
	e.artifacts[id] = a
	// The index is the local snapshot manifest; the public target.snapshot
	// remains the element JPEG so older application cards keep working.
	raw, marshalErr := json.Marshal(e.artifacts)
	if marshalErr == nil {
		file := filepath.Join(e.dir, "artifacts", ".index-"+rand.Text())
		if os.WriteFile(file, raw, 0600) == nil {
			_ = os.Rename(file, filepath.Join(e.dir, "artifacts", "index.json"))
		}
		_ = os.Remove(file)
	}
	e.mu.Unlock()
	return id, nil
}
func (s *session) registerDownload(guid string) {
	if !idPattern.MatchString(guid) {
		return
	}
	path := filepath.Join(s.e.dir, "downloads", s.id, guid)
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4<<20 {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_, _ = s.e.storeArtifact(s.scope, "application/octet-stream", data)
}

// Never call Element.Screenshot here: its scroll/RAF helpers can use Rod's
// unbounded root Page and hang when a popup backgrounds the opener.
func passiveElementScreenshot(page proto.Client, id proto.RuntimeRemoteObjectID, quality int) ([]byte, error) {
	shape, err := (proto.DOMGetContentQuads{ObjectID: id}).Call(page)
	if err != nil {
		return nil, err
	}
	box := shape.Box()
	if box.Width <= 0 || box.Height <= 0 || box.X < 0 || box.Y < 0 {
		return nil, errors.New("target_not_found")
	}
	// Quads and Clip use CSS pixels. Let Chrome apply device scale rather
	// than cropping device pixels with CSS coordinates on Retina/Windows.
	view, err := (proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatJpeg, Quality: &quality, FromSurface: true, Clip: &proto.PageViewport{X: box.X, Y: box.Y, Width: math.Min(box.Width, 4096), Height: math.Min(box.Height, 4096), Scale: 1}}).Call(page)
	if err != nil {
		return nil, err
	}
	return view.Data, nil
}

// Full-page evidence must use the document bounds, rather than silently
// recording only the viewport. Keep the legacy sensitive-page guard.
func (e *engine) fullSnapshot(owner string, p *rod.Page) (string, error) {
	shape, err := p.Eval(snapshotSource)
	if err != nil {
		return "", err
	}
	if shape.Value.Get("unsafe").Bool() {
		return "", errors.New("sensitive_snapshot")
	}
	metrics, err := (proto.PageGetLayoutMetrics{}).Call(p)
	if err != nil {
		return "", err
	}
	box := metrics.CSSContentSize
	if box == nil || box.Width <= 0 || box.Height <= 0 || box.Width*box.Height > 32<<20 {
		return "", errors.New("artifact_too_large")
	}
	shot, err := (proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng, FromSurface: true, CaptureBeyondViewport: true, Clip: &proto.PageViewport{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height, Scale: 1}}).Call(p)
	if err != nil {
		return "", err
	}
	return e.storeArtifact(owner, "image/png", shot.Data)
}
