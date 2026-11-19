package bot

import (
	"testing"

	"github.com/meetbot/meetbot/internal/cdp"
)

func TestClassifyPage(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *cdp.PageSnapshot
		expected MeetState
	}{
		{
			name: "Pantalla con botón unirse",
			snapshot: &cdp.PageSnapshot{
				URL:     "https://meet.google.com/abc-defg-hij",
				Title:   "Google Meet",
				Buttons: []string{"Unirse ahora"},
			},
			expected: StatePrejoin,
		},
		{
			name: "Sala de espera",
			snapshot: &cdp.PageSnapshot{
				URL:     "https://meet.google.com/abc-defg-hij",
				Title:   "Google Meet",
				Text:    "Alguien te dejará entrar pronto a la llamada",
				Buttons: []string{"Cancelar"},
			},
			expected: StateWaiting,
		},
		{
			name: "Dentro de la reunión",
			snapshot: &cdp.PageSnapshot{
				URL:     "https://meet.google.com/abc-defg-hij",
				Title:   "Google Meet",
				Buttons: []string{"Salir de la llamada", "Silenciar micrófono"},
			},
			expected: StateJoined,
		},
		{
			name: "Reunión finalizada",
			snapshot: &cdp.PageSnapshot{
				URL:     "https://meet.google.com/abc-defg-hij",
				Title:   "Google Meet",
				Text:    "Has salido de la reunión",
				Buttons: []string{"Volver a la pantalla principal"},
			},
			expected: StateEnded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPage(tt.snapshot)
			if got != tt.expected {
				t.Errorf("ClassifyPage() = %v, esperado %v", got, tt.expected)
			}
		})
	}
}

func TestDecideAction(t *testing.T) {
	snap := &cdp.PageSnapshot{
		Inputs: []cdp.InputFound{
			{Index: 0, Label: "Tu nombre", Value: ""},
		},
		Buttons: []string{"Unirse ahora"},
	}

	act := DecideAction(snap, "Bot Asistente")
	if act.Kind != "type" || act.Text != "Bot Asistente" {
		t.Errorf("Se esperaba que escribiera el nombre, obtuvo: %+v", act)
	}

	snap.Inputs[0].Value = "Bot Asistente"
	act2 := DecideAction(snap, "Bot Asistente")
	if act2.Kind != "click" || act2.Text != "Unirse ahora" {
		t.Errorf("Se esperaba que pulsara 'Unirse ahora', obtuvo: %+v", act2)
	}
}
