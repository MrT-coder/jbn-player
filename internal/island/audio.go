//go:build windows

package island

import (
	"jbnplayer/internal/ipc"
	"jbnplayer/internal/player"
)

// audioResult is the audio settings (volume, device, equalizer) plus an error.
type audioResult struct {
	player.AudioInfo
	Error string `json:"error"`
}

func failedAudio(err error) audioResult {
	return audioResult{AudioInfo: player.AudioInfo{Devices: []player.Device{}}, Error: err.Error()}
}

func okAudio(a player.AudioInfo) audioResult {
	if a.Devices == nil {
		a.Devices = []player.Device{}
	}
	return audioResult{AudioInfo: a}
}

// AudioInfo reads the audio settings from the player.
func (s *service) AudioInfo() audioResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return failedAudio(err)
	}
	a, err := cl.Audio()
	if err != nil {
		return failedAudio(err)
	}
	return okAudio(a)
}

// AudioDo changes one audio setting: volume (req.Level), device (req.Name) or eq (req.EQ).
func (s *service) AudioDo(op string, req ipc.Request) audioResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return failedAudio(err)
	}
	a, err := cl.DoAudio(op, req)
	if err != nil {
		return failedAudio(err)
	}
	return okAudio(a)
}
