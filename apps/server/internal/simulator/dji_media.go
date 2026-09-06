package simulator

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type DJIMediaController interface {
	Start(context.Context, string, string) error
	Stop(string) error
	Close() error
}

type FFmpegMediaController struct {
	executable   string
	hostOverride string
	mu           sync.Mutex
	processes    map[string]*exec.Cmd
}

func NewFFmpegMediaController(executable, hostOverride string) (*FFmpegMediaController, error) {
	resolved, err := exec.LookPath(strings.TrimSpace(executable))
	if err != nil {
		return nil, errors.New("DJI_SIMULATOR_FFMPEG_NOT_FOUND")
	}
	return &FFmpegMediaController{executable: resolved, hostOverride: strings.TrimSpace(hostOverride), processes: map[string]*exec.Cmd{}}, nil
}

func mediaPattern(videoID string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(videoID))
	if hash.Sum32()%2 == 0 {
		return "testsrc2=size=1280x720:rate=15"
	}
	return "smptebars=size=1280x720:rate=15"
}

func overrideMediaHost(raw, hostname string) (string, error) {
	destination, err := url.Parse(raw)
	if err != nil || (destination.Scheme != "rtmp" && destination.Scheme != "rtmps") || destination.Hostname() == "" {
		return "", errors.New("DJI_SIMULATOR_RTMP_URL_INVALID")
	}
	if hostname == "" {
		return destination.String(), nil
	}
	port := destination.Port()
	destination.Host = hostname
	if port != "" {
		destination.Host += ":" + port
	}
	return destination.String(), nil
}

func (controller *FFmpegMediaController) Start(ctx context.Context, videoID, rawDestination string) error {
	if strings.TrimSpace(videoID) == "" {
		return errors.New("DJI_SIMULATOR_VIDEO_ID_REQUIRED")
	}
	destination, err := overrideMediaHost(rawDestination, controller.hostOverride)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if _, exists := controller.processes[videoID]; exists {
		return nil
	}
	command := exec.CommandContext(ctx, controller.executable,
		"-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", mediaPattern(videoID),
		"-an", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-pix_fmt", "yuv420p", "-g", "30", "-f", "flv", destination,
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return err
	}
	controller.processes[videoID] = command
	go func() {
		_ = command.Wait()
		controller.mu.Lock()
		if controller.processes[videoID] == command {
			delete(controller.processes, videoID)
		}
		controller.mu.Unlock()
	}()
	return nil
}

func (controller *FFmpegMediaController) Stop(videoID string) error {
	controller.mu.Lock()
	command := controller.processes[videoID]
	delete(controller.processes, videoID)
	controller.mu.Unlock()
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Signal(os.Interrupt)
}

func (controller *FFmpegMediaController) Close() error {
	controller.mu.Lock()
	commands := make([]*exec.Cmd, 0, len(controller.processes))
	for videoID, command := range controller.processes {
		commands = append(commands, command)
		delete(controller.processes, videoID)
	}
	controller.mu.Unlock()
	for _, command := range commands {
		if command.Process != nil {
			_ = command.Process.Signal(os.Interrupt)
		}
	}
	return nil
}
