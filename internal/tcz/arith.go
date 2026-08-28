package tcz

// 自适应二进制算术编码器 (无进位传播变体, lpaq 风格)。
// 12 位概率, 32 位区间, 按 8 位对齐刷新。

const pScale = 4096

// Prob 是某上下文中 bit=1 的自适应概率 (12 位)。
type Prob uint16

func newProb() Prob { return 2048 }

// update 向实际 bit 方向自适应调整。shift 越大适应越慢。
func (p *Prob) update(bit int, shift uint) {
	if bit != 0 {
		*p += Prob((pScale - int(*p)) >> shift)
	} else {
		*p -= Prob(int(*p) >> shift)
	}
}

type Encoder struct {
	x1, x2 uint32
	out    *BitWriter
}

func NewEncoder(out *BitWriter) *Encoder {
	return &Encoder{x1: 0, x2: 0xffffffff, out: out}
}

// Encode 以 p (P(bit=1), 0..4096) 编码一个 bit。
// p 被截断到 [1,4095], 保证区间严格收缩。
func (e *Encoder) Encode(bit int, p Prob) {
	if p < 1 {
		p = 1
	} else if p > 4095 {
		p = 4095
	}
	xmid := e.x1 + uint32(uint64(e.x2-e.x1)*uint64(p)>>12)
	if xmid >= e.x2 {
		xmid = e.x2 - 1 // 防御性截断
	}
	if bit != 0 {
		e.x2 = xmid
	} else {
		e.x1 = xmid + 1
	}
	// 重归一化: 顶部字节一致即移出 (lpaq1 语义)。
	// 注意不能加 x1 != x2 守卫: 区间可能收缩到 x1==x2 (range=1 且 p 极端时),
	// 此时恰恰需要移出字节并把区间重新展开为 [v<<8, v<<8|255],
	// 否则区间冻死在一个点, 后续编码进入倒置区间, 编解码静默分歧。
	for (e.x1^e.x2)&0xff000000 == 0 {
		e.out.WriteBits(uint64(e.x2>>24), 8)
		e.x1 <<= 8
		e.x2 = e.x2<<8 | 255
	}
}

// Flush 写出最终区间任一点的 4 字节。
func (e *Encoder) Flush() {
	e.out.WriteBits(uint64(e.x1>>24), 8)
	e.out.WriteBits(uint64(e.x1>>16)&255, 8)
	e.out.WriteBits(uint64(e.x1>>8)&255, 8)
	e.out.WriteBits(uint64(e.x1)&255, 8)
}

type Decoder struct {
	x1, x2 uint32
	x      uint32
	in     *BitReader
	eof    bool
}

func NewDecoder(in *BitReader) *Decoder {
	d := &Decoder{x1: 0, x2: 0xffffffff, in: in}
	for i := 0; i < 4; i++ {
		d.x = d.x<<8 | uint32(in.readByte())
	}
	return d
}

// Decode 以 p 返回解码出的 bit, 模型更新由调用方完成。
func (d *Decoder) Decode(p Prob) int {
	if p < 1 {
		p = 1
	} else if p > 4095 {
		p = 4095
	}
	xmid := d.x1 + uint32(uint64(d.x2-d.x1)*uint64(p)>>12)
	if xmid >= d.x2 {
		xmid = d.x2 - 1
	}
	bit := 1
	if d.x > xmid {
		bit = 0
	}
	if bit != 0 {
		d.x2 = xmid
	} else {
		d.x1 = xmid + 1
	}
	// 与编码器对称的重归一化 (含 x1==x2 区间塌缩的再展开, 见 Encode 注释)。
	for (d.x1^d.x2)&0xff000000 == 0 {
		d.x1 <<= 8
		d.x2 = d.x2<<8 | 255
		d.x = d.x<<8 | uint32(d.in.readByte())
	}
	return bit
}

// BitWriter 按 MSB-first 写入任意位宽。
type BitWriter struct {
	buf  []byte
	acc  uint64
	nbit uint
}

func (w *BitWriter) WriteBits(v uint64, n uint) {
	w.acc = w.acc<<n | (v & ((1 << n) - 1))
	w.nbit += n
	for w.nbit >= 8 {
		w.buf = append(w.buf, byte(w.acc>>(w.nbit-8)))
		w.nbit -= 8
		w.acc &= (1 << w.nbit) - 1
	}
}

// Align 字节对齐, 补零。
func (w *BitWriter) Align() []byte {
	if w.nbit > 0 {
		w.buf = append(w.buf, byte(w.acc<<(8-w.nbit)))
		w.acc = 0
		w.nbit = 0
	}
	return w.buf
}

// BitReader 按 MSB-first 读取。
type BitReader struct {
	buf []byte
	pos int
	bit uint
}

func NewBitReader(buf []byte) *BitReader {
	return &BitReader{buf: buf}
}

// readByte 从位流中取下一字节; 越界返回 0 (容错, 解码端配合大小上限)。
func (r *BitReader) readByte() byte {
	if r.pos >= len(r.buf) {
		return 0
	}
	b := r.buf[r.pos] << r.bit
	if r.bit > 0 && r.pos+1 < len(r.buf) {
		b |= r.buf[r.pos+1] >> (8 - r.bit)
	}
	r.pos++
	return b
}

// ReadBit 读单个位。
func (r *BitReader) ReadBit() int {
	if r.pos >= len(r.buf) {
		return 0
	}
	bit := int(r.buf[r.pos] >> (7 - r.bit) & 1)
	r.bit++
	if r.bit == 8 {
		r.bit = 0
		r.pos++
	}
	return bit
}
