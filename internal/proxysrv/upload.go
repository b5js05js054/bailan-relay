package proxysrv

import (
	"fmt"
	"net/http"
	"sync"

	"bailan-relay/internal/bodydelta"
)

// uploadBaseLimit 客户端保留的基准请求体数量（Devin 常有多个对话/子代理交替请求）。
const uploadBaseLimit = 10

// uploadPlan 一次转发实际上传的内容：raw=原样；full=整包 gzip；delta=相对 base 的增量再 gzip。
type uploadPlan struct {
	payload []byte
	mode    string
	gzip    bool
	base    *bodydelta.Index
	sum     string
}

func (p uploadPlan) apply(h http.Header) {
	h.Set(bodydelta.HeaderMode, p.mode)
	h.Set(bodydelta.HeaderSum, p.sum)
	if p.gzip {
		h.Set(bodydelta.HeaderEncoding, "gzip")
	}
	if p.base != nil {
		h.Set(bodydelta.HeaderBase, p.base.Hash)
	}
}

func (p uploadPlan) describe(total int) string {
	switch p.mode {
	case bodydelta.ModeDelta:
		return fmt.Sprintf("增量上传 %s（省 %d%%）", humanBytes(len(p.payload)), savedPercent(len(p.payload), total))
	case bodydelta.ModeFull:
		return fmt.Sprintf("压缩上传 %s（省 %d%%）", humanBytes(len(p.payload)), savedPercent(len(p.payload), total))
	}
	return "原始上传"
}

// uploader 记录中继是否支持增量上传，以及中继已缓存、可作为基准的最近请求体。
type uploader struct {
	mu        sync.Mutex
	relay     string
	supported bool
	bases     []*bodydelta.Index
}

func (u *uploader) plan(relay string, body []byte) uploadPlan {
	sum := bodydelta.Sum(body)
	u.mu.Lock()
	if u.relay != relay {
		u.relay, u.supported, u.bases = relay, false, nil
	}
	supported, bases := u.supported, u.bases
	u.mu.Unlock()

	if !supported || len(body) < bodydelta.MinSize {
		return uploadPlan{payload: body, mode: bodydelta.ModeRaw, sum: sum}
	}
	if ix, delta := bodydelta.Best(bases, body); ix != nil {
		return uploadPlan{payload: bodydelta.Gzip(delta), mode: bodydelta.ModeDelta, gzip: true, base: ix, sum: sum}
	}
	return fullPlan(body, sum)
}

func fullPlan(body []byte, sum string) uploadPlan {
	return uploadPlan{payload: bodydelta.Gzip(body), mode: bodydelta.ModeFull, gzip: true, sum: sum}
}

// fallback 中继拒收本次上传方式时给出重发方案；nil 表示无需重发。
func (u *uploader) fallback(resp *http.Response, plan uploadPlan, body []byte) (*uploadPlan, string) {
	support := resp.Header.Get(bodydelta.HeaderSupport)
	switch {
	case plan.mode == bodydelta.ModeDelta && resp.StatusCode == http.StatusConflict && support == "miss":
		u.mu.Lock()
		kept := u.bases[:0:0]
		for _, ix := range u.bases {
			if ix != plan.base {
				kept = append(kept, ix)
			}
		}
		u.bases = kept
		u.mu.Unlock()
		next := fullPlan(body, plan.sum)
		return &next, "中继没有该增量基准（可能刚重启），改为整包上传"
	case plan.mode != bodydelta.ModeRaw && support == "" && resp.StatusCode >= 400:
		u.mu.Lock()
		u.supported, u.bases = false, nil
		u.mu.Unlock()
		return &uploadPlan{payload: body, mode: bodydelta.ModeRaw, sum: plan.sum}, "中继版本不支持压缩上传，改为原始上传"
	}
	return nil, ""
}

// observe 根据中继响应头更新支持状态；中继确认已缓存本次请求体时把它加入基准。
func (u *uploader) observe(relay string, resp *http.Response, plan uploadPlan, body []byte) {
	supported := resp.Header.Get(bodydelta.HeaderSupport) != ""
	stored := supported && resp.Header.Get(bodydelta.HeaderStored) == plan.sum
	resp.Header.Del(bodydelta.HeaderSupport)
	resp.Header.Del(bodydelta.HeaderStored)
	var latest *bodydelta.Index
	if stored {
		latest = bodydelta.NewIndex(body)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.relay != relay {
		return
	}
	u.supported = supported
	if !supported {
		u.bases = nil
	} else if latest != nil {
		u.bases = bodydelta.Remember(u.bases, plan.base, latest, uploadBaseLimit)
	}
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

func savedPercent(sent, total int) int {
	if total <= 0 || sent >= total {
		return 0
	}
	return (total - sent) * 100 / total
}
