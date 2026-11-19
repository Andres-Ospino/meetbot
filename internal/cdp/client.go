package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Browser representa una conexión activa con Chrome mediante DevTools Protocol (CDP).
type Browser interface {
	Snapshot(ctx context.Context) (*PageSnapshot, error)
	Click(ctx context.Context, text string) (bool, error)
	Type(ctx context.Context, inputIndex int, text string) error
	Screenshot(ctx context.Context) (string, error)
	EnableCaptions(ctx context.Context) bool
	CaptionSpeaker(ctx context.Context) (speaker string, chars int, err error)
	Participants(ctx context.Context) ([]string, error)
	SendChat(ctx context.Context, text string) (bool, error)
	Close() error
}

type PageSnapshot struct {
	URL     string       `json:"url"`
	Title   string       `json:"title"`
	Text    string       `json:"text"`
	Buttons []string     `json:"buttons"`
	Inputs  []InputFound `json:"inputs"`
}

type InputFound struct {
	Index int    `json:"index"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type Client struct {
	wsURL string
	conn  *websocket.Conn
	mu    sync.Mutex
	msgID uint64
}

// Connect se conecta al WebSocket CDP de Chrome en la dirección dada (ej. localhost:9222).
func Connect(ctx context.Context, addr string) (*Client, error) {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	resp, err := http.Get(addr + "/json/list")
	if err != nil {
		return nil, fmt.Errorf("error conectando a Chrome CDP (%s): %w", addr, err)
	}
	defer resp.Body.Close()

	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("error decodificando targets de Chrome: %w", err)
	}

	wsURL := ""
	for _, t := range targets {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" {
			wsURL = t.WebSocketDebuggerURL
			break
		}
	}
	if wsURL == "" && len(targets) > 0 {
		wsURL = targets[0].WebSocketDebuggerURL
	}
	if wsURL == "" {
		return nil, errors.New("no se encontró ninguna página activa en Chrome")
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error al abrir WebSocket CDP: %w", err)
	}

	return &Client{wsURL: wsURL, conn: conn}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := atomic.AddUint64(&c.msgID, 1)
	req := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}

	if err := c.conn.WriteJSON(req); err != nil {
		return fmt.Errorf("fallo enviando comando CDP %s: %w", method, err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	_ = c.conn.SetReadDeadline(deadline)

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("fallo leyendo respuesta CDP %s: %w", method, err)
		}
		var raw struct {
			ID     uint64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(msg, &raw); err != nil {
			continue
		}
		if raw.ID != id {
			continue
		}
		if raw.Error != nil {
			return fmt.Errorf("error CDP %s: %s", method, raw.Error.Message)
		}
		if out != nil && len(raw.Result) > 0 {
			return json.Unmarshal(raw.Result, out)
		}
		return nil
	}
}

func (c *Client) eval(ctx context.Context, expr string, out any) error {
	var res struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
	}
	err := c.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
	}, &res)
	if err != nil {
		return err
	}
	if out != nil {
		raw, _ := json.Marshal(res.Result.Value)
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (c *Client) Snapshot(ctx context.Context) (*PageSnapshot, error) {
	const script = `(() => {
		const norm = s => (s || '').replace(/\s+/g, ' ').trim();
		const buttons = [];
		document.querySelectorAll('button, [role="button"], input[type="button"], input[type="submit"]').forEach(el => {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) {
				const text = norm(el.innerText || el.getAttribute('aria-label') || el.title || el.value);
				if (text && !buttons.includes(text)) buttons.push(text);
			}
		});
		const inputs = [];
		let idx = 0;
		document.querySelectorAll('input:not([type="hidden"]):not([type="submit"]):not([type="button"]):not([type="checkbox"]):not([type="radio"])').forEach(el => {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) {
				const label = norm(el.getAttribute('aria-label') || el.placeholder || el.title || '');
				inputs.push({ index: idx++, label, value: el.value || '' });
			}
		});
		return {
			url: window.location.href,
			title: document.title,
			text: norm(document.body ? document.body.innerText : '').slice(0, 4000),
			buttons: buttons.slice(0, 50),
			inputs: inputs
		};
	})()`

	var snap PageSnapshot
	if err := c.eval(ctx, script, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (c *Client) Click(ctx context.Context, text string) (bool, error) {
	script := fmt.Sprintf(`(() => {
		const target = %q.toLowerCase().trim();
		const norm = s => (s || '').replace(/\s+/g, ' ').toLowerCase().trim();
		const matches = [];
		document.querySelectorAll('button, [role="button"], input[type="button"], input[type="submit"], a').forEach(el => {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) {
				const t = norm(el.innerText || el.getAttribute('aria-label') || el.title || el.value);
				if (t === target || (target.length >= 3 && t.includes(target))) {
					matches.push(el);
				}
			}
		});
		if (matches.length > 0) {
			matches[0].scrollIntoView({ block: 'center' });
			matches[0].click();
			return true;
		}
		return false;
	})()`, text)

	var clicked bool
	err := c.eval(ctx, script, &clicked)
	return clicked, err
}

func (c *Client) Type(ctx context.Context, inputIndex int, text string) error {
	script := fmt.Sprintf(`(() => {
		let idx = 0;
		let found = null;
		document.querySelectorAll('input:not([type="hidden"]):not([type="submit"]):not([type="button"])').forEach(el => {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) {
				if (idx === %d) found = el;
				idx++;
			}
		});
		if (found) {
			found.focus();
			found.value = %q;
			found.dispatchEvent(new Event('input', { bubbles: true }));
			found.dispatchEvent(new Event('change', { bubbles: true }));
			return true;
		}
		return false;
	})()`, inputIndex, text)

	var ok bool
	if err := c.eval(ctx, script, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("campo input en índice %d no encontrado", inputIndex)
	}
	return nil
}

func (c *Client) Screenshot(ctx context.Context) (string, error) {
	var res struct {
		Data string `json:"data"`
	}
	err := c.call(ctx, "Page.captureScreenshot", map[string]any{"format": "jpeg", "quality": 60}, &res)
	if err != nil {
		return "", err
	}
	return res.Data, nil
}

func (c *Client) EnableCaptions(ctx context.Context) bool {
	const script = `(() => {
		const norm = s => (s || '').toLowerCase();
		let ok = false;
		document.querySelectorAll('button, [role="button"]').forEach(el => {
			const t = norm(el.getAttribute('aria-label') || el.innerText || el.title);
			if (t.includes('turn on captions') || t.includes('activar subtitulos') || t.includes('subtitulos')) {
				el.click();
				ok = true;
			}
		});
		return ok;
	})()`
	var ok bool
	_ = c.eval(ctx, script, &ok)
	return ok
}

func (c *Client) CaptionSpeaker(ctx context.Context) (string, int, error) {
	const script = `(() => {
		const nodes = document.querySelectorAll('[aria-live], .caption-line, .captions-text, [class*="caption"]');
		let lastSpeaker = '';
		let totalChars = 0;
		nodes.forEach(el => {
			const t = (el.innerText || '').trim();
			if (t) {
				totalChars += t.length;
				const m = t.match(/^([^:\n]{2,30}):/);
				if (m) lastSpeaker = m[1].trim();
			}
		});
		return { speaker: lastSpeaker, chars: totalChars };
	})()`
	var res struct {
		Speaker string `json:"speaker"`
		Chars   int    `json:"chars"`
	}
	err := c.eval(ctx, script, &res)
	return res.Speaker, res.Chars, err
}

func (c *Client) Participants(ctx context.Context) ([]string, error) {
	const script = `(() => {
		const names = new Set();
		const norm = s => (s || '').replace(/\s+/g, ' ').trim();
		document.querySelectorAll('[role="listitem"], [data-participant-id], [class*="participant"], [class*="member"]').forEach(el => {
			const t = norm(el.innerText || el.getAttribute('aria-label') || '');
			if (t && t.length > 2 && t.length < 50 && !t.includes('\n')) {
				names.add(t);
			}
		});
		return Array.from(names);
	})()`
	var names []string
	err := c.eval(ctx, script, &names)
	return names, err
}

func (c *Client) SendChat(ctx context.Context, text string) (bool, error) {
	script := fmt.Sprintf(`(() => {
		const norm = s => (s || '').toLowerCase();
		let chatBtn = null;
		document.querySelectorAll('button, [role="button"]').forEach(el => {
			const t = norm(el.getAttribute('aria-label') || el.innerText || el.title);
			if (t.includes('chat') || t.includes('mensaje')) chatBtn = el;
		});
		if (chatBtn) chatBtn.click();
		setTimeout(() => {
			const input = document.querySelector('textarea, input[type="text"][aria-label*="chat"], [contenteditable="true"]');
			if (input) {
				input.focus();
				if (input.tagName === 'TEXTAREA' || input.tagName === 'INPUT') {
					input.value = %q;
					input.dispatchEvent(new Event('input', { bubbles: true }));
				}
				const send = document.querySelector('button[aria-label*="send"], button[aria-label*="enviar"]');
				if (send) send.click();
			}
		}, 300);
		return true;
	})()`, text)
	var ok bool
	err := c.eval(ctx, script, &ok)
	return ok, err
}
