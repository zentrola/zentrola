package gateway

import "encoding/json"

const responsesScanCaptureLimit = 64 << 10

// responsesEventScanner 以有界内存流式扫描超过 SSE 事件上限的 Responses 事件，
// 只抽取顶层 type 与 response.usage。response.completed 可能携带包含图像
// base64 的完整 output，整体缓存会被丢弃并导致 usage 丢失。
type responsesEventScanner struct {
	stack []byte
	// keys 保存第 1、2 层对象当前成员的键；更深层级与判定无关，不记录。
	keys                       [2]string
	inString, escape           bool
	keyString, keyOverflow     bool
	wantKey, wantValue         bool
	keyRaw                     []byte
	capture                    []byte
	capturing                  bool
	captureDepth               int
	captureUsage               bool
	kind                       string
	usage                      []byte
	started, complete, invalid bool
	usageInvalid               bool
}

func (s *responsesEventScanner) feed(p []byte) {
	for index := 0; index < len(p); index++ {
		if s.invalid {
			return
		}
		// 未捕获的普通字符串值（如 base64）只需寻找结束引号与转义符。
		if s.inString && !s.keyString && !s.capturing && !s.escape {
			for index < len(p) && p[index] != '"' && p[index] != '\\' && p[index] >= 0x20 {
				index++
			}
			if index == len(p) {
				return
			}
		}
		s.byte(p[index])
	}
}

func (s *responsesEventScanner) byte(b byte) {
	if s.complete {
		if !jsonSpace(b) {
			s.invalid = true
		}
		return
	}
	if s.capturing {
		if len(s.capture) >= responsesScanCaptureLimit {
			s.invalid = true
			return
		}
		s.capture = append(s.capture, b)
	}
	if s.inString {
		s.stringByte(b)
		return
	}
	if jsonSpace(b) {
		return
	}
	switch b {
	case '"':
		if s.top() == '{' && s.wantKey {
			s.keyString, s.keyOverflow = true, false
			s.keyRaw = append(s.keyRaw[:0], b)
		} else {
			s.valueStart(b)
		}
		s.inString = true
	case '{', '[':
		if len(s.stack) == 0 {
			if b != '{' || s.started {
				s.invalid = true
				return
			}
			s.started = true
		} else {
			s.valueStart(b)
		}
		if len(s.stack) >= 256 {
			s.invalid = true
			return
		}
		s.stack = append(s.stack, b)
		s.wantKey = b == '{'
	case '}', ']':
		top := s.top()
		if (b == '}' && top != '{') || (b == ']' && top != '[') {
			s.invalid = true
			return
		}
		s.stack = s.stack[:len(s.stack)-1]
		s.wantKey, s.wantValue = false, false
		if s.capturing && len(s.stack) == s.captureDepth {
			s.finishCapture()
		}
		if len(s.stack) == 0 {
			s.complete = true
		}
	case ':':
		if s.top() != '{' {
			s.invalid = true
			return
		}
		s.wantValue = true
	case ',':
		if len(s.stack) == 0 {
			s.invalid = true
			return
		}
		s.wantKey = s.top() == '{'
	default:
		if len(s.stack) == 0 {
			s.invalid = true
			return
		}
		s.valueStart(b)
	}
}

func (s *responsesEventScanner) stringByte(b byte) {
	if s.keyString {
		if len(s.keyRaw) > 128 {
			s.keyOverflow = true
		} else {
			s.keyRaw = append(s.keyRaw, b)
		}
	}
	if s.escape {
		s.escape = false
		return
	}
	if b == '\\' {
		s.escape = true
		return
	}
	if b < 0x20 {
		s.invalid = true
		return
	}
	if b != '"' {
		return
	}
	s.inString = false
	if s.keyString {
		s.keyString, s.wantKey = false, false
		key := ""
		// 超长键不可能是 type / response / usage，仅记录为空键继续扫描。
		if !s.keyOverflow && json.Unmarshal(s.keyRaw, &key) != nil {
			s.invalid = true
			return
		}
		if depth := len(s.stack); depth <= len(s.keys) {
			s.keys[depth-1] = key
		}
		return
	}
	if s.capturing && len(s.stack) == s.captureDepth {
		s.finishCapture()
	}
}

// valueStart 在对象成员值的第一个字节处判断是否命中目标路径。
func (s *responsesEventScanner) valueStart(b byte) {
	if !s.wantValue {
		return
	}
	s.wantValue = false
	depth := len(s.stack)
	switch {
	case depth == 1 && s.keys[0] == "type":
		if b != '"' {
			s.invalid = true
			return
		}
		s.beginCapture(b, false)
	case depth == 2 && s.keys[0] == "response" && s.keys[1] == "usage":
		switch b {
		case '{':
			s.beginCapture(b, true)
		case 'n':
		default:
			s.usageInvalid = true
		}
	}
}

func (s *responsesEventScanner) beginCapture(b byte, usage bool) {
	s.capturing = true
	s.captureUsage = usage
	s.captureDepth = len(s.stack)
	s.capture = append(s.capture[:0], b)
}

func (s *responsesEventScanner) finishCapture() {
	s.capturing = false
	if s.captureUsage {
		s.usage = append(s.usage[:0], s.capture...)
	} else if json.Unmarshal(s.capture, &s.kind) != nil {
		s.invalid = true
	}
	s.capture = s.capture[:0]
}

func (s *responsesEventScanner) top() byte {
	if len(s.stack) == 0 {
		return 0
	}
	return s.stack[len(s.stack)-1]
}

// result 返回事件类型与 usage 原文；JSON 结构不完整或非法时 ok 为 false。
func (s *responsesEventScanner) result() (string, []byte, bool) {
	if !s.started || !s.complete || s.invalid {
		return "", nil, false
	}
	return s.kind, s.usage, true
}

func jsonSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
