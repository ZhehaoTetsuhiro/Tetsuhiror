package tcz

// tcz — Tetsuhiro Compressor: 自研高压缩比格式。
//
// 第一级: LZ77 变体 (TLZ)。
//   - 32 KiB 滑动窗口, 最小匹配 4 字节, 最大 2048。
//   - 贪心 + 惰性匹配双策略: 若下一位置存在更长匹配,
//     则当前字节降级为 literal, 换取整体更优的 token 流。
//   - 哈希链加速候选查找, 每位置至多探测 64 条链。
//   - 输出 token 流: literal 与 (距离, 长度) 匹配交替。

const (
	windowSize = 1 << 15
	windowMask = windowSize - 1
	minMatch   = 4
	maxMatch   = 2048
	hashBits   = 16
	hashSize   = 1 << hashBits
	maxChain   = 64
	lazyChain  = 16
)

type hashTable struct {
	head []int32
	prev []int32
}

func newHashTable() *hashTable {
	h := &hashTable{
		head: make([]int32, hashSize),
		prev: make([]int32, windowSize),
	}
	h.reset()
	return h
}

func (h *hashTable) reset() {
	for i := range h.head {
		h.head[i] = -1
	}
	for i := range h.prev {
		h.prev[i] = -1
	}
}

func hash3(d []byte, i int) int {
	v := uint32(d[i])<<16 | uint32(d[i+1])<<8 | uint32(d[i+2])
	return int((v * 2654435761) >> (32 - hashBits))
}

// insert 将位置 i 插入哈希链 (要求 i+3 <= n)。
func (h *hashTable) insert(d []byte, i, n int) {
	if i+3 > n {
		return
	}
	hv := hash3(d, i)
	idx := i & windowMask
	h.prev[idx] = h.head[hv]
	h.head[hv] = int32(i)
}

func (h *hashTable) next(cand int32) int32 {
	return h.prev[cand&int32(windowMask)]
}

// matchLen 计算两位置起的最长公共前缀, 上限 maxLen。
func matchLen(d []byte, s, c, maxLen int) int {
	if maxLen <= 0 || d[s] != d[c] {
		return 0
	}
	n := 0
	for n < maxLen && d[s+n] == d[c+n] {
		n++
	}
	return n
}

// findBest 返回 pos 处的最长匹配; 返回长度 0 表示无匹配。
func findBest(ht *hashTable, d []byte, pos, n int, chainLimit int) Match {
	var best Match
	if pos+minMatch > n {
		return best
	}
	limit := n - pos
	if limit > maxMatch {
		limit = maxMatch
	}
	cand := ht.head[hash3(d, pos)]
	for chain := 0; cand >= 0 && chain < chainLimit; chain++ {
		c := int(cand)
		dist := pos - c
		if dist <= 0 || dist > windowSize {
			break
		}
		if d[c] == d[pos] && d[c+1] == d[pos+1] && d[c+2] == d[pos+2] {
			if l := matchLen(d, pos, c, limit); l > best.Len {
				best = Match{Dist: dist, Len: l}
				if l >= limit {
					break
				}
			}
		}
		cand = ht.next(cand)
	}
	return best
}

// Match 是一个 LZ 匹配 token。
type Match struct {
	Dist int
	Len  int
}

// 压缩主循环: 生成 token 序列。
// tokens[i] 为 true 表示 literal[i]; 为 false 表示 matches[k]。
func compressLZ(data []byte) (literals []byte, matches []Match, tokens []byte) {
	n := len(data)
	tokens = make([]byte, 0, n/2+8)
	matches = make([]Match, 0, n/8+8)

	ht := newHashTable()

	for i := 0; i < n; {
		best := findBest(ht, data, i, n, maxChain)

		if best.Len >= minMatch {
			// 惰性匹配: 若 i+1 处存在更长匹配, 当前字节降级为 literal。
			if best.Len < 64 && i+1+minMatch <= n {
				next := findBest(ht, data, i+1, n, lazyChain)
				if next.Len > best.Len {
					ht.insert(data, i, n)
					tokens = append(tokens, 1)
					literals = append(literals, data[i])
					i++
					continue
				}
			}
			ht.insert(data, i, n)
			matches = append(matches, best)
			tokens = append(tokens, 0)
			end := i + best.Len
			for j := i + 1; j < end && j+3 <= n; j++ {
				ht.insert(data, j, n)
			}
			i = end
		} else {
			ht.insert(data, i, n)
			tokens = append(tokens, 1)
			literals = append(literals, data[i])
			i++
		}
	}
	return literals, matches, tokens
}

// decompressLZ 按 token 流重建原始数据。
func decompressLZ(literals []byte, matches []Match, tokens []byte, origLen int) []byte {
	out := make([]byte, 0, origLen)
	li, mi := 0, 0
	for _, t := range tokens {
		if t == 1 {
			out = append(out, literals[li])
			li++
		} else {
			m := matches[mi]
			mi++
			start := len(out) - m.Dist
			for k := 0; k < m.Len; k++ {
				out = append(out, out[start+k])
			}
		}
	}
	return out
}
