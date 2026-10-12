// Package bodydelta 中继请求体增量编码：同一对话相邻两次请求绝大部分字节相同，
// 客户端只上传相对上一份请求的差异（rsync 式块匹配），中继还原后与原始请求逐字节一致。
//
// server-go/internal/bodydelta 与 bailan-relay-src/internal/bodydelta 是同一份代码，
// 线上格式（magic / op 编码）改动必须两边同步。
package bodydelta

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
)

// HTTP 头约定。
const (
	HeaderMode     = "X-Bailan-Body"     // raw / full / delta；带此头表示客户端支持增量，中继会缓存请求体
	HeaderEncoding = "X-Bailan-Encoding" // gzip
	HeaderBase     = "X-Bailan-Base"     // delta 基准请求体的 sha256
	HeaderSum      = "X-Bailan-Sha256"   // 还原后请求体的 sha256
	HeaderSupport  = "X-Bailan-Delta"    // 中继响应：1=支持；miss=基准缺失或校验失败，请整包重发
	HeaderStored   = "X-Bailan-Stored"   // 中继响应：已缓存该 sha256 的请求体，可作为下次的基准

	ModeRaw   = "raw"
	ModeFull  = "full"
	ModeDelta = "delta"
)

const (
	// MinSize 小于此大小的请求体不做增量，也不缓存。
	MinSize = 64 << 10
	// MaxSize 还原 / 解压后的请求体上限，防止压缩炸弹。
	MaxSize = 128 << 20

	blockSize = 2048
	hashMul   = 1099511628211
	opCopy    = 1
	opInsert  = 2
)

var magic = []byte("BLD1")

var errCorrupt = errors.New("bodydelta: 增量数据损坏")

// Sum 返回请求体 sha256（十六进制）。
func Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Index 基准请求体的块索引，可复用于多次 Encode。
type Index struct {
	Hash   string
	Base   []byte
	blocks map[uint64]int
}

func NewIndex(base []byte) *Index {
	ix := &Index{Hash: Sum(base), Base: base, blocks: make(map[uint64]int, len(base)/blockSize+1)}
	for off := 0; off+blockSize <= len(base); off += blockSize {
		h := hashBlock(base[off : off+blockSize])
		if _, dup := ix.blocks[h]; !dup {
			ix.blocks[h] = off
		}
	}
	return ix
}

var rollPow = func() uint64 {
	p := uint64(1)
	for i := 0; i < blockSize-1; i++ {
		p *= hashMul
	}
	return p
}()

func hashBlock(b []byte) uint64 {
	var h uint64
	for _, c := range b {
		h = h*hashMul + uint64(c) + 1
	}
	return h
}

func roll(h uint64, out, in byte) uint64 {
	return (h-(uint64(out)+1)*rollPow)*hashMul + uint64(in) + 1
}

func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	i := 0
	for i+8 <= n && binary.LittleEndian.Uint64(a[i:]) == binary.LittleEndian.Uint64(b[i:]) {
		i += 8
	}
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// Encode 生成 target 相对基准的增量；literal 为需要原样上传的字节数（越小越划算）。
// maxLiteral > 0 时新内容一旦超过该值就放弃（ok=false），用于在多个基准里快速淘汰不相关的。
func (ix *Index) Encode(target []byte, maxLiteral int) (payload []byte, literal int, ok bool) {
	out := append([]byte(nil), magic...)
	litStart, pos := 0, 0
	var h uint64
	hashed := false
	flush := func(end int) {
		if end > litStart {
			out = append(out, opInsert)
			out = binary.AppendUvarint(out, uint64(end-litStart))
			out = append(out, target[litStart:end]...)
			literal += end - litStart
		}
	}
	for pos+blockSize <= len(target) {
		if maxLiteral > 0 && literal+pos-litStart > maxLiteral {
			return nil, literal + pos - litStart, false
		}
		if !hashed {
			h = hashBlock(target[pos : pos+blockSize])
			hashed = true
		}
		if off, ok := ix.blocks[h]; ok && bytes.Equal(ix.Base[off:off+blockSize], target[pos:pos+blockSize]) {
			back := 0
			for back < pos-litStart && back < off && target[pos-back-1] == ix.Base[off-back-1] {
				back++
			}
			start, baseStart := pos-back, off-back
			n := back + blockSize + commonPrefix(target[pos+blockSize:], ix.Base[off+blockSize:])
			flush(start)
			out = append(out, opCopy)
			out = binary.AppendUvarint(out, uint64(baseStart))
			out = binary.AppendUvarint(out, uint64(n))
			pos = start + n
			litStart = pos
			hashed = false
			continue
		}
		if pos+blockSize < len(target) {
			h = roll(h, target[pos], target[pos+blockSize])
		}
		pos++
	}
	flush(len(target))
	if maxLiteral > 0 && literal > maxLiteral {
		return nil, literal, false
	}
	return out, literal, true
}

// Best 在多个基准里挑新内容最少的一个；都不划算（新内容超过一半）时返回 nil。
func Best(bases []*Index, target []byte) (best *Index, payload []byte) {
	limit := len(target) / 2
	for _, ix := range bases {
		if p, literal, ok := ix.Encode(target, limit); ok {
			best, payload, limit = ix, p, max(literal, 1)
		}
	}
	return best, payload
}

// Remember 把刚发出的请求体放到基准列表最前面，刚用过的基准紧随其后（不同对话可能共用同一基准，
// 不能直接替换掉），超过 limit 时淘汰最久未用的。
func Remember(bases []*Index, used, latest *Index, limit int) []*Index {
	out := make([]*Index, 0, limit)
	out = append(out, latest)
	if used != nil && used.Hash != latest.Hash && limit > 1 {
		out = append(out, used)
	}
	for _, ix := range bases {
		if ix != used && ix.Hash != latest.Hash && len(out) < limit {
			out = append(out, ix)
		}
	}
	return out
}

// Apply 用基准还原请求体；结果超过 maxSize 或数据越界时报错。
func Apply(base, payload []byte, maxSize int) ([]byte, error) {
	if !bytes.HasPrefix(payload, magic) {
		return nil, errCorrupt
	}
	p := payload[len(magic):]
	out := make([]byte, 0, len(base)+len(p))
	readUvarint := func() (uint64, bool) {
		v, n := binary.Uvarint(p)
		if n <= 0 {
			return 0, false
		}
		p = p[n:]
		return v, true
	}
	for len(p) > 0 {
		op := p[0]
		p = p[1:]
		switch op {
		case opCopy:
			off, ok1 := readUvarint()
			n, ok2 := readUvarint()
			if !ok1 || !ok2 || off > uint64(len(base)) || n > uint64(len(base))-off || n > uint64(maxSize-len(out)) {
				return nil, errCorrupt
			}
			out = append(out, base[off:off+n]...)
		case opInsert:
			n, ok := readUvarint()
			if !ok || n > uint64(len(p)) || n > uint64(maxSize-len(out)) {
				return nil, errCorrupt
			}
			out = append(out, p[:n]...)
			p = p[n:]
		default:
			return nil, errCorrupt
		}
	}
	return out, nil
}

// Gzip 以最快档压缩（大请求体在上传前压缩，CPU 开销远小于省下的上传时间）。
func Gzip(b []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

// Gunzip 解压并限制解压后大小。
func Gunzip(b []byte, maxSize int) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, int64(maxSize)+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxSize {
		return nil, errors.New("bodydelta: 解压后超过大小上限")
	}
	return out, nil
}
