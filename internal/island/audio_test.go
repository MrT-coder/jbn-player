//go:build windows

package island

import (
	"testing"

	"jbnplayer/internal/ipc"
)

func TestAudioBridgeReadsAndChangesVolume(t *testing.T) {
	s := libService(t)
	stop, err := ipc.Serve(s.ipcPath, &ctl{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	a := s.AudioInfo()
	if a.Error != "" || len(a.Devices) != 1 || a.Devices[0].Name != "auto" {
		t.Fatalf("info: %+v", a)
	}
	if a = s.AudioDo("volume", ipc.Request{Level: 30}); a.Error != "" {
		t.Fatalf("volume: %+v", a)
	}
	if st := s.PlayerState(); st.Volume != 30 {
		t.Fatalf("the light state must carry the volume for the slider: %+v", st)
	}
}

func TestAudioWhenPlayerNotRunningReportsErrorWithUsableLists(t *testing.T) {
	s := libService(t) // no ipc server
	a := s.AudioInfo()
	if a.Error != "player is not running" || a.Devices == nil {
		t.Fatalf("got %+v", a)
	}
}
