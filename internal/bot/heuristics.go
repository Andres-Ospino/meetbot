package bot

import (
	"regexp"
	"strings"

	"github.com/Andres-Ospino/meetbot/internal/cdp"
)

type MeetState string

const (
	StatePrejoin MeetState = "prejoin" // Pantalla de configuración previa (cámara/mic/nombre)
	StateWaiting MeetState = "waiting" // Esperando que el anfitrión admita al bot en sala de espera
	StateJoined  MeetState = "joined"  // Dentro de la reunión
	StateDenied  MeetState = "denied"  // Rechazado o enlace inválido
	StateEnded   MeetState = "ended"   // La llamada terminó o sacaron al bot
	StateOffsite MeetState = "offsite" // Página fuera de la reunión (login o advertencia de cuenta)
	StateUnknown MeetState = "unknown"
)

type Action struct {
	Kind  string // "type" | "click" | "wait"
	Input int    // índice de input para "type"
	Text  string // texto a escribir o botón a pulsar
}

var (
	offsiteHosts = regexp.MustCompile(`^https?://(accounts\.google\.com|workspace\.google\.com|login\.microsoftonline\.com|login\.live\.com|appleid\.apple\.com|([a-z0-9-]+\.)?okta\.com|([a-z0-9-]+\.)?auth0\.com)(/|$)`)

	joinedButtons = []string{
		"leave call", "leave meeting", "salir de la llamada", "abandonar la llamada",
		"salir de la reunion", "colgar", "hang up", "leave",
	}

	waitingTexts = []string{
		"asking to join", "solicitando unirse", "pidiendo unirte", "pidiendo unirse",
		"waiting for the host", "esperando a que el organizador", "someone will let you in",
		"alguien te dejara entrar", "te dejaran entrar", "you'll join the call when someone lets you in",
		"please wait", "espera a que", "lobby", "sala de espera", "waiting room",
	}

	deniedTexts = []string{
		"you can't join", "no puedes unirte", "denied your request", "rechazo tu solicitud",
		"no one responded", "nadie respondio", "check your meeting code", "revisa el codigo",
		"invalid meeting", "this meeting has been locked", "la reunion esta bloqueada",
	}

	endedTexts = []string{
		"finalizo la reunion para todos", "ended the meeting for everyone", "you left the meeting",
		"has salido de la reunion", "saliste de la reunion", "the meeting has ended",
		"la reunion ha finalizado", "la reunion termino", "you've been removed", "te quitaron",
		"call ended", "llamada finalizada", "the host has ended", "rejoin", "volver a unirse",
	}

	joinButtons = []string{
		"join now", "unirse ahora", "unirme ahora", "ask to join", "pedir unirse",
		"pedir unirme", "solicitar unirse", "solicitar unirme", "join meeting",
		"unirse a la reunion", "join", "unirse", "participar",
	}

	proceedButtons = []string{
		"continue without microphone", "continuar sin microfono", "continue without",
		"continuar sin", "continue on this browser", "continuar en este navegador",
		"join from your browser", "unirse desde el navegador", "use the web app",
		"usar la aplicacion web", "launch meeting", "iniciar reunion", "got it", "entendido",
		"dismiss", "aceptar", "ok",
	}

	nameLabels = []string{"name", "nombre", "your name", "tu nombre"}

	neverButtons = []string{
		"iniciar sesion", "sign in", "acceder", "log in", "login", "crear cuenta", "create account",
		"cambiar de cuenta", "switch account", "mas informacion", "learn more", "ayuda", "help",
		"descargar", "download", "instalar", "install", "abrir la app", "open app", "abrir zoom",
		"open zoom", "abrir microsoft teams", "open microsoft teams", "enviar comentarios", "send feedback",
	}

	rejoinButtons = []string{"rejoin", "volver a unirse", "volver a unirte", "volver a unirme"}
)

func fold(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "’", "'")
	return r.Replace(strings.ToLower(s))
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

func ClassifyPage(p *cdp.PageSnapshot) MeetState {
	if p == nil {
		return StateUnknown
	}
	u := strings.ToLower(strings.TrimSpace(p.URL))
	if strings.HasPrefix(u, "http") && offsiteHosts.MatchString(u) {
		return StateOffsite
	}

	text := fold(p.Title + "\n" + p.Text)
	var buttons []string
	for _, b := range p.Buttons {
		buttons = append(buttons, fold(b))
	}

	hasButton := func(needles []string) bool {
		for _, b := range buttons {
			if containsAny(b, needles) {
				return true
			}
		}
		return false
	}

	switch {
	case containsAny(text, endedTexts):
		return StateEnded
	case containsAny(text, deniedTexts):
		return StateDenied
	case containsAny(text, waitingTexts):
		return StateWaiting
	case hasButton(joinedButtons) && !hasButton(joinButtons):
		return StateJoined
	case hasButton(joinButtons) || hasButton(proceedButtons) || len(p.Inputs) > 0:
		return StatePrejoin
	}
	return StateUnknown
}

func DecideAction(p *cdp.PageSnapshot, botName string) Action {
	// 1. Rellenar nombre si el campo está vacío
	for _, in := range p.Inputs {
		if strings.TrimSpace(in.Value) == "" && containsAny(fold(in.Label), nameLabels) {
			return Action{Kind: "type", Input: in.Index, Text: botName}
		}
	}

	// 2. Pulsar botón para avanzar o unirse
	for _, group := range [][]string{joinButtons, proceedButtons} {
		for _, want := range group {
			for _, b := range p.Buttons {
				fb := fold(b)
				if containsAny(fb, neverButtons) || (want == "ok" && fb != "ok") {
					continue
				}
				if strings.Contains(fb, want) && !containsAny(fb, joinedButtons) && !containsAny(fb, rejoinButtons) {
					return Action{Kind: "click", Text: b}
				}
			}
		}
	}

	return Action{Kind: "wait"}
}

func IsInCall(p *cdp.PageSnapshot) bool {
	if p == nil {
		return false
	}
	for _, b := range p.Buttons {
		fb := fold(b)
		if containsAny(fb, joinedButtons) && !containsAny(fb, rejoinButtons) {
			return true
		}
	}
	return false
}
