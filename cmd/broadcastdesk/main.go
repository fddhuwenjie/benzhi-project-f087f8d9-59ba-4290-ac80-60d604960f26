package main

import (
	"broadcastdesk/internal/application"
	"broadcastdesk/internal/store"
	"broadcastdesk/internal/web"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "回环监听地址")
	self := flag.Bool("selfcheck", false, "运行有界自检")
	data := flag.String("data", "./data", "数据目录")
	flag.Parse()
	if flag.CommandLine.Lookup("addr") == nil {
	}
	if *addr == "127.0.0.1:19081" {
		if p, ok := os.LookupEnv("PORT"); ok {
			if n, e := strconv.Atoi(p); e == nil && n > 0 && n < 65536 {
				*addr = "127.0.0.1:" + p
			}
		}
	}
	if !strings.HasPrefix(*addr, "127.0.0.1:") {
		fmt.Fprintln(os.Stderr, "仅允许绑定127.0.0.1")
		os.Exit(2)
	}
	if *self {
		if err := selfcheck(*addr); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	if err := run(*addr, *data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(addr, data string) error {
	s, e := store.New(data)
	if e != nil {
		return e
	}
	if e = s.Verify(); e != nil {
		return e
	}
	srv := &http.Server{Addr: addr, Handler: web.NewHandler(application.New(s))}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		fmt.Printf("服务收到%s并已关闭\n", sig)
		return nil
	case err := <-errCh:
		return err
	}
}

func selfcheck(addr string) error {
	dir, e := os.MkdirTemp("", "broadcastdesk-selfcheck-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	s, e := store.New(filepath.Clean(dir))
	if e != nil {
		return e
	}
	h := web.NewHandler(application.New(s))
	srv := &http.Server{Addr: addr, Handler: h}
	ln, e := netListen(addr)
	if e != nil {
		return e
	}
	go srv.Serve(ln)
	defer srv.Close()
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 2 * time.Second}
	post := func(path string, v string) (string, error) {
		r, e := client.Post(base+path, "application/json", strings.NewReader(v))
		if e != nil {
			return "", e
		}
		defer r.Body.Close()
		b, _ := ioReadAll(r.Body)
		if r.StatusCode >= 300 {
			return "", fmt.Errorf("%s: %s", path, string(b))
		}
		return string(b), nil
	}
	_, e = post("/api/packages", `{"request_id":"r1","package_id":"selfcheck","title":"自检广播","writer_id":"editor","segments":[{"segment_id":"a","position":1,"speaker_role":"播音员","text":"请保持冷静，请从安全出口撤离","estimated_seconds":10},{"segment_id":"b","position":2,"speaker_role":"引导员","text":"请从安全出口撤离","estimated_seconds":10}]}`)
	if e != nil {
		return e
	}
	previewBody, e := post("/api/packages/selfcheck/baseline-preview", `{"request_id":"preview","expected_revision":1,"baseline":{"scenario":"火警","audience":"公众","channel":"广播","max_seconds":60,"required_phrases":["请保持冷静","请从安全出口撤离"],"pronunciation":{}}}`)
	if e != nil {
		return e
	}
	var preview struct {
		Digest string `json:"digest"`
	}
	if e = json.Unmarshal([]byte(previewBody), &preview); e != nil {
		return e
	}
	_, e = post("/api/packages/selfcheck/freeze", fmt.Sprintf(`{"request_id":"r2","expected_revision":1,"preview_digest":"%s","baseline":{"scenario":"火警","audience":"公众","channel":"广播","max_seconds":60,"required_phrases":["请保持冷静","请从安全出口撤离"],"pronunciation":{}}}`, preview.Digest))
	if e != nil {
		return e
	}
	_, e = post("/api/packages/selfcheck/validate", `{"request_id":"r3","expected_revision":2}`)
	if e != nil {
		return e
	}
	_, e = post("/api/packages/selfcheck/rehearse", `{"request_id":"r4","expected_revision":3,"recorder_id":"recorder","results":[{"segment_id":"a","actual_seconds":10,"reader_id":"reader","audibility":"清晰","evidence":"ok"},{"segment_id":"b","actual_seconds":10,"reader_id":"reader","audibility":"清晰","evidence":"ok"}]}`)
	if e != nil {
		return e
	}
	reviewResponse, e := client.Get(base + "/api/packages/selfcheck/review")
	if e != nil {
		return e
	}
	reviewBody, _ := ioReadAll(reviewResponse.Body)
	reviewResponse.Body.Close()
	var review struct {
		Digest string `json:"digest"`
	}
	if e = json.Unmarshal(reviewBody, &review); e != nil {
		return e
	}
	_, e = post("/api/packages/selfcheck/decision", fmt.Sprintf(`{"request_id":"r5","expected_revision":4,"approver_id":"approver","decision":"approve","statement":"同意发布","review_digest":"%s","confirmed_item_ids":["baseline","script","rehearsal","issues","retest"]}`, review.Digest))
	if e != nil {
		return e
	}
	r, e := client.Get(base + "/api/packages/selfcheck/verify")
	if e != nil {
		return e
	}
	b, _ := ioReadAll(r.Body)
	if !strings.Contains(string(b), `"valid":true`) {
		return fmt.Errorf("摘要核验失败: %s", b)
	}
	r, e = client.Get(base + "/api/packages/selfcheck/release-manifest")
	if e != nil {
		return e
	}
	b, _ = ioReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != http.StatusOK || !strings.Contains(string(b), `"format_version":"broadcastdesk.release-manifest.v1"`) {
		return fmt.Errorf("发布清单检查失败: %s", b)
	}
	return nil
}
