package media

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Recorder struct {
	audioCmd *exec.Cmd
	videoCmd *exec.Cmd
	audioOut string
	videoOut string
}

func NewRecorder(audioPath, videoPath string) *Recorder {
	return &Recorder{
		audioOut: audioPath,
		videoOut: videoPath,
	}
}

// StartAudio graba el audio desde la tarjeta de sonido virtual de PulseAudio usando ffmpeg.
func (r *Recorder) StartAudio(sinkMonitor string) error {
	if sinkMonitor == "" {
		sinkMonitor = "reunion.monitor"
	}
	if err := os.MkdirAll(filepath.Dir(r.audioOut), 0755); err != nil {
		return err
	}

	// ffmpeg -f pulse -i reunion.monitor -c:a libopus -b:a 48k -y output.opus
	cmd := exec.Command("ffmpeg",
		"-f", "pulse",
		"-i", sinkMonitor,
		"-c:a", "libopus",
		"-b:a", "48k",
		"-y",
		r.audioOut,
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error al iniciar grabación de audio con ffmpeg: %w", err)
	}
	r.audioCmd = cmd
	log.Printf("[recorder] Grabación de audio iniciada en %s", r.audioOut)
	return nil
}

// StartVideo graba la pantalla X11 virtual (DISPLAY=:99) con ffmpeg.
func (r *Recorder) StartVideo(display string) error {
	if display == "" {
		display = ":99"
	}
	if err := os.MkdirAll(filepath.Dir(r.videoOut), 0755); err != nil {
		return err
	}

	// ffmpeg -f x11grab -draw_mouse 0 -framerate 10 -i :99 -c:v libx264 -preset ultrafast -crf 28 -y output.mp4
	cmd := exec.Command("ffmpeg",
		"-f", "x11grab",
		"-draw_mouse", "0",
		"-framerate", "10",
		"-i", display,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "28",
		"-y",
		r.videoOut,
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error al iniciar grabación de video: %w", err)
	}
	r.videoCmd = cmd
	log.Printf("[recorder] Grabación de video iniciada en %s", r.videoOut)
	return nil
}

// Stop finaliza limpiamente los procesos de grabación enviando SIGINT para cerrar los archivos correctamente.
func (r *Recorder) Stop() {
	stopProcess := func(cmd *exec.Cmd, name string) {
		if cmd != nil && cmd.Process != nil {
			log.Printf("[recorder] Deteniendo grabación de %s...", name)
			_ = cmd.Process.Signal(syscall.SIGINT)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case <-done:
			case <-time.After(4 * time.Second):
				_ = cmd.Process.Kill()
			}
		}
	}

	stopProcess(r.audioCmd, "audio")
	stopProcess(r.videoCmd, "video")
}

// ReadAudio lee el archivo de audio resultante.
func (r *Recorder) ReadAudio() ([]byte, error) {
	return os.ReadFile(r.audioOut)
}

// ReadVideo lee el archivo de video si existe.
func (r *Recorder) ReadVideo() ([]byte, error) {
	if r.videoOut == "" {
		return nil, nil
	}
	return os.ReadFile(r.videoOut)
}
